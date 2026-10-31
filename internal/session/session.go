package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// CookieName 是浏览器携带会话令牌的 cookie 名，授权端点与 API 共用。
	CookieName = "relay_session"
	keyPrefix  = "relay:session:"
	// userPrefix 存该用户当前有效的会话令牌集合，用于一次性下线全部设备。
	userPrefix = "relay:user-sessions:"
	DefaultTTL = 12 * time.Hour
)

var ErrNotFound = errors.New("session: 会话不存在或已过期")

type Manager struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(rdb *redis.Client) *Manager {
	return &Manager{rdb: rdb, ttl: DefaultTTL}
}

func (m *Manager) Create(ctx context.Context, userID string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}

	pipe := m.rdb.TxPipeline()
	pipe.Set(ctx, keyPrefix+token, userID, m.ttl)
	pipe.SAdd(ctx, userPrefix+userID, token)
	pipe.Expire(ctx, userPrefix+userID, m.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("session: 写入会话: %w", err)
	}
	return token, nil
}

func (m *Manager) Resolve(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrNotFound
	}
	userID, err := m.rdb.Get(ctx, keyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("session: 读取会话: %w", err)
	}
	return userID, nil
}

func (m *Manager) TTL() time.Duration {
	return m.ttl
}

func (m *Manager) Destroy(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	userID, err := m.rdb.Get(ctx, keyPrefix+token).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("session: 读取会话: %w", err)
	}

	pipe := m.rdb.TxPipeline()
	pipe.Del(ctx, keyPrefix+token)
	if err == nil {
		pipe.SRem(ctx, userPrefix+userID, token)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("session: 删除会话: %w", err)
	}
	return nil
}

// RevokeAll 清掉该用户当前所有会话，返回失效的令牌数量。
func (m *Manager) RevokeAll(ctx context.Context, userID string) (int, error) {
	key := userPrefix + userID
	tokens, err := m.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("session: 读取用户会话集合: %w", err)
	}

	pipe := m.rdb.TxPipeline()
	for _, token := range tokens {
		pipe.Del(ctx, keyPrefix+token)
	}
	pipe.Del(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("session: 清空用户会话: %w", err)
	}
	return len(tokens), nil
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("session: 生成令牌: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
