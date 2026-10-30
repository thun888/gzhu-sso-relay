package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/thun888/gzhu-sso-relay/internal/auth"
	"github.com/thun888/gzhu-sso-relay/internal/cache"
	"github.com/thun888/gzhu-sso-relay/internal/config"
	"github.com/thun888/gzhu-sso-relay/internal/cryptox"
	"github.com/thun888/gzhu-sso-relay/internal/httpapi"
	"github.com/thun888/gzhu-sso-relay/internal/session"
	"github.com/thun888/gzhu-sso-relay/internal/store"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	key, err := cryptox.ParseKey(cfg.Secret)
	if err != nil {
		return err
	}
	if _, err := cryptox.NewCipher(key); err != nil {
		return err
	}

	db, err := store.Open(cfg.Database.DSN)
	if err != nil {
		return err
	}
	if err := store.Migrate(db); err != nil {
		return err
	}

	rdb := cache.Open(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer rdb.Close()

	authService := auth.NewService(auth.Options{
		Users:    repo.NewUserRepo(db),
		Accounts: repo.NewAccountRepo(db),
		Sessions: session.New(rdb),
		Throttle: auth.NewThrottle(rdb),
		Upstream: auth.Upstream{
			CASBaseURL:    cfg.CAS.BaseURL,
			PortalBaseURL: cfg.Portal.BaseURL,
			Timeout:       30 * time.Second,
		},
		EmailDomain: cfg.EmailDomain,
	})
	authHandler := httpapi.NewAuth(authService, strings.HasPrefix(cfg.BaseURL, "https://"))

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/healthz", healthz(db, rdb))
	router.POST("/api/login", authHandler.Login)
	router.POST("/api/logout", authHandler.Logout)
	router.GET("/api/me", authHandler.RequireUser(), authHandler.Me)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("relay 监听 %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

type healthStatus struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Redis    string `json:"redis"`
}

func healthz(db *gorm.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		result := healthStatus{Status: "ok", Database: "ok", Redis: "ok"}
		code := http.StatusOK

		if err := pingDatabase(ctx, db); err != nil {
			result.Database = "unavailable"
			result.Status = "degraded"
			code = http.StatusServiceUnavailable
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			result.Redis = "unavailable"
			result.Status = "degraded"
			code = http.StatusServiceUnavailable
		}

		c.JSON(code, result)
	}
}

func pingDatabase(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
