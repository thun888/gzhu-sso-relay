package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const captchaPrefix = "relay:captcha:"

// CaptchaGuard 记录登录失败过的来访 IP，窗口期内这些 IP 必须先过验证码。
type CaptchaGuard struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewCaptchaGuard(rdb *redis.Client, ttl time.Duration) *CaptchaGuard {
	return &CaptchaGuard{rdb: rdb, ttl: ttl}
}

func (g *CaptchaGuard) Required(ctx context.Context, ip string) (bool, error) {
	n, err := g.rdb.Exists(ctx, captchaPrefix+ip).Result()
	if err != nil {
		return false, fmt.Errorf("auth: 读取验证码标记: %w", err)
	}
	return n > 0, nil
}

func (g *CaptchaGuard) Flag(ctx context.Context, ip string) error {
	if err := g.rdb.Set(ctx, captchaPrefix+ip, time.Now().Unix(), g.ttl).Err(); err != nil {
		return fmt.Errorf("auth: 写入验证码标记: %w", err)
	}
	return nil
}

func (g *CaptchaGuard) Clear(ctx context.Context, ip string) error {
	if err := g.rdb.Del(ctx, captchaPrefix+ip).Err(); err != nil {
		return fmt.Errorf("auth: 清除验证码标记: %w", err)
	}
	return nil
}
