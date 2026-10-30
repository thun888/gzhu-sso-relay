package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	throttlePrefix = "relay:authfail:"
	maxAttempts    = 5
	throttleWindow = 15 * time.Minute
)

// Throttle 限制同一账号触发上游验证的频率，避免本地密码校验失败时被用来反复试上游。
type Throttle struct {
	rdb *redis.Client
}

func NewThrottle(rdb *redis.Client) *Throttle {
	return &Throttle{rdb: rdb}
}

func (t *Throttle) Allow(ctx context.Context, subject string) (bool, error) {
	key := throttlePrefix + subject
	count, err := t.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("auth: 读取尝试计数: %w", err)
	}
	if count == 1 {
		if err := t.rdb.Expire(ctx, key, throttleWindow).Err(); err != nil {
			return false, fmt.Errorf("auth: 设置尝试计数过期: %w", err)
		}
	}
	return count <= maxAttempts, nil
}

func (t *Throttle) Reset(ctx context.Context, subject string) error {
	return t.rdb.Del(ctx, throttlePrefix+subject).Err()
}
