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
	keyPrefix  = "relay:session:"
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
	if err := m.rdb.Set(ctx, keyPrefix+token, userID, m.ttl).Err(); err != nil {
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
	if err := m.rdb.Del(ctx, keyPrefix+token).Err(); err != nil {
		return fmt.Errorf("session: 删除会话: %w", err)
	}
	return nil
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("session: 生成令牌: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
