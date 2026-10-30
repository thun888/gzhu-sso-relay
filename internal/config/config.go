package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr    string
	BaseURL     string
	EmailDomain string
	Database    Database
	Redis       Redis
	Secret      string
	CAS         Endpoint
	Portal      Endpoint
}

type Database struct {
	DSN string
}

type Redis struct {
	Addr     string
	Password string
	DB       int
}

type Endpoint struct {
	BaseURL string
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:    env("RELAY_HTTP_ADDR", ":8080"),
		BaseURL:     strings.TrimSuffix(env("RELAY_BASE_URL", "http://localhost:8080"), "/"),
		EmailDomain: env("RELAY_EMAIL_DOMAIN", "gzhu.edu.cn"),
		Database:    Database{DSN: env("RELAY_DB_DSN", "")},
		Redis: Redis{
			Addr:     env("RELAY_REDIS_ADDR", "localhost:6379"),
			Password: env("RELAY_REDIS_PASSWORD", ""),
		},
		Secret: env("RELAY_SECRET_KEY", ""),
		CAS:    Endpoint{BaseURL: env("RELAY_CAS_BASE_URL", "https://newcas.gzhu.edu.cn")},
		Portal: Endpoint{BaseURL: env("RELAY_PORTAL_BASE_URL", "https://newmy.gzhu.edu.cn")},
	}

	redisDB, err := strconv.Atoi(env("RELAY_REDIS_DB", "0"))
	if err != nil {
		return nil, fmt.Errorf("配置 RELAY_REDIS_DB 不是整数: %w", err)
	}
	cfg.Redis.DB = redisDB

	var missing []string
	if cfg.Database.DSN == "" {
		missing = append(missing, "RELAY_DB_DSN")
	}
	if cfg.Secret == "" {
		missing = append(missing, "RELAY_SECRET_KEY")
	}
	if len(missing) > 0 {
		return nil, errors.New("缺少必需配置: " + strings.Join(missing, ", "))
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
