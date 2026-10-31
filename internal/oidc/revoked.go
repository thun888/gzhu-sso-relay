package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const revokedPrefix = "relay:revoked:"

// RevokedStore 记录已撤销的访问令牌。JWT 本身无状态，只能按令牌摘要登记到过期为止。
type RevokedStore struct {
	rdb *redis.Client
}

func NewRevokedStore(rdb *redis.Client) *RevokedStore {
	return &RevokedStore{rdb: rdb}
}

func (s *RevokedStore) Revoke(ctx context.Context, token string, ttl time.Duration) error {
	if token == "" || ttl <= 0 {
		return nil
	}
	if err := s.rdb.Set(ctx, revokedPrefix+digest(token), "1", ttl).Err(); err != nil {
		return fmt.Errorf("oidc: 登记撤销令牌: %w", err)
	}
	return nil
}

func (s *RevokedStore) Revoked(ctx context.Context, token string) (bool, error) {
	n, err := s.rdb.Exists(ctx, revokedPrefix+digest(token)).Result()
	if err != nil {
		return false, fmt.Errorf("oidc: 查询撤销记录: %w", err)
	}
	return n > 0, nil
}

// digest 让 Redis 里不出现令牌原文。
func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
