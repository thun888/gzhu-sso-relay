package oidc

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	tokenFailPrefix = "relay:tokenfail:"
	tokenFailLimit  = 20
	tokenFailWindow = 5 * time.Minute
)

// TokenLimiter 按来源 IP 统计令牌端点的失败次数。
// 错误 client_secret 会触发 Argon2id 校验，不限流时单个来源就能把 CPU 打满。
type TokenLimiter struct {
	rdb *redis.Client
}

func NewTokenLimiter(rdb *redis.Client) *TokenLimiter {
	return &TokenLimiter{rdb: rdb}
}

func (l *TokenLimiter) Blocked(ctx context.Context, ip string) (bool, error) {
	if ip == "" {
		return false, nil
	}
	count, err := l.rdb.Get(ctx, tokenFailPrefix+ip).Int()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return count >= tokenFailLimit, nil
}

func (l *TokenLimiter) Hit(ctx context.Context, ip string) error {
	if ip == "" {
		return nil
	}
	key := tokenFailPrefix + ip
	pipe := l.rdb.TxPipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, tokenFailWindow)
	_, err := pipe.Exec(ctx)
	return err
}
