package main

import (
	"context"
	"errors"
	"flag"
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
	"github.com/thun888/gzhu-sso-relay/internal/oidc"
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
	path := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}

	key, err := cryptox.ParseKey(cfg.SecretKey)
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
	if err := store.Seed(db); err != nil {
		return err
	}

	rdb := cache.Open(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer rdb.Close()

	keys, err := oidc.LoadOrCreateKeys(cfg.Server.SigningKeyPath)
	if err != nil {
		return err
	}

	users := repo.NewUserRepo(db)
	accounts := repo.NewAccountRepo(db)
	clients := repo.NewClientRepo(db)
	groups := repo.NewGroupRepo(db)
	sessions := session.New(rdb)

	authService := auth.NewService(auth.Options{
		Users:    users,
		Accounts: accounts,
		Sessions: sessions,
		Throttle: auth.NewThrottle(rdb),
		Upstream: auth.Upstream{
			CASBaseURL:    cfg.Upstream.CASBaseURL,
			PortalBaseURL: cfg.Upstream.PortalBaseURL,
			Timeout:       cfg.Upstream.Timeout.Std(),
		},
		EmailDomain: cfg.Server.EmailDomain,
	})
	secure := strings.HasPrefix(cfg.Server.BaseURL, "https://")
	authHandler := httpapi.NewAuth(authService, secure)
	loginPage, err := httpapi.NewLoginPage(authService, secure)
	if err != nil {
		return err
	}

	provider := oidc.New(oidc.Options{
		Issuer:   cfg.Server.BaseURL,
		LoginURL: cfg.Server.LoginURL,
		Keys:     keys,
		Clients:  clients,
		Accounts: accounts,
		Users:    users,
		Sessions: sessions,
		Codes:    oidc.NewCodeStore(rdb),
	})

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/healthz", healthz(db, rdb))
	router.GET("/.well-known/openid-configuration", provider.Discovery)
	router.GET("/oidc/jwks", provider.JWKS)
	router.GET("/oidc/authorize", provider.Authorize)
	router.POST("/oidc/token", provider.Token)
	router.GET("/oidc/userinfo", provider.UserInfo)
	router.POST("/api/login", authHandler.Login)
	router.POST("/api/logout", authHandler.Logout)
	router.GET("/api/me", authHandler.RequireUser(), authHandler.Me)
	router.GET("/login", loginPage.Show)
	router.POST("/login", loginPage.Submit)

	adminHandler := httpapi.NewAdmin(users, groups)
	admin := router.Group("/api/admin", authHandler.RequireAdmin())
	admin.GET("/users", adminHandler.ListUsers)
	admin.PATCH("/users/:id", adminHandler.PatchUser)
	admin.GET("/groups", adminHandler.ListGroups)
	admin.POST("/groups", adminHandler.CreateGroup)
	admin.PATCH("/groups/:id", adminHandler.PatchGroup)
	admin.DELETE("/groups/:id", adminHandler.DeleteGroup)
	admin.GET("/groups/:id/members", adminHandler.GroupMembers)
	admin.PUT("/groups/:id/members", adminHandler.SetGroupMembers)

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("relay 监听 %s", cfg.Server.Addr)
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
