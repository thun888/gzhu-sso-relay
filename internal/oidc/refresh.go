package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	refreshPrefix = "relay:refresh:"
	// refreshUserPrefix 存该用户当前有效的刷新令牌集合，禁用账号时一次性清空。
	refreshUserPrefix = "relay:user-refresh:"
	refreshTTL        = 30 * 24 * time.Hour
)

var ErrRefreshInvalid = errors.New("oidc: 刷新令牌无效或已使用")

type RefreshGrant struct {
	UserID   string   `json:"user_id"`
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes"`
	AuthTime int64    `json:"auth_time"`
}

type RefreshStore struct {
	rdb *redis.Client
}

func NewRefreshStore(rdb *redis.Client) *RefreshStore {
	return &RefreshStore{rdb: rdb}
}

func (s *RefreshStore) Issue(ctx context.Context, grant RefreshGrant) (string, error) {
	raw, err := json.Marshal(grant)
	if err != nil {
		return "", fmt.Errorf("oidc: 序列化刷新令牌: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("oidc: 生成刷新令牌: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)

	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, refreshPrefix+token, raw, refreshTTL)
	pipe.SAdd(ctx, refreshUserPrefix+grant.UserID, token)
	pipe.Expire(ctx, refreshUserPrefix+grant.UserID, refreshTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("oidc: 写入刷新令牌: %w", err)
	}
	return token, nil
}

// Peek 读取刷新令牌但不删除，供令牌端点先完成客户端与用户状态校验。
func (s *RefreshStore) Peek(ctx context.Context, token string) (*RefreshGrant, error) {
	if token == "" {
		return nil, ErrRefreshInvalid
	}

	raw, err := s.rdb.Get(ctx, refreshPrefix+token).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrRefreshInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("oidc: 读取刷新令牌: %w", err)
	}

	var grant RefreshGrant
	if err := json.Unmarshal(raw, &grant); err != nil {
		return nil, fmt.Errorf("oidc: 解析刷新令牌: %w", err)
	}
	return &grant, nil
}

// Consume 原子取出并删除刷新令牌，同一次授权只能换回一次令牌，轮转后旧值立即作废。
func (s *RefreshStore) Consume(ctx context.Context, token string) (*RefreshGrant, error) {
	if token == "" {
		return nil, ErrRefreshInvalid
	}

	raw, err := s.rdb.GetDel(ctx, refreshPrefix+token).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrRefreshInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("oidc: 取出刷新令牌: %w", err)
	}

	var grant RefreshGrant
	if err := json.Unmarshal(raw, &grant); err != nil {
		return nil, fmt.Errorf("oidc: 解析刷新令牌: %w", err)
	}
	// 索引里残留的成员不影响校验，交给集合自身的 TTL 清理。
	_ = s.rdb.SRem(ctx, refreshUserPrefix+grant.UserID, token)
	return &grant, nil
}

// Revoke 删除刷新令牌并报告是否命中。
func (s *RefreshStore) Revoke(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}

	grant, err := s.Peek(ctx, token)
	if errors.Is(err, ErrRefreshInvalid) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	n, err := s.rdb.Del(ctx, refreshPrefix+token).Result()
	if err != nil {
		return false, fmt.Errorf("oidc: 撤销刷新令牌: %w", err)
	}
	if n > 0 {
		_ = s.rdb.SRem(ctx, refreshUserPrefix+grant.UserID, token)
	}
	return n > 0, nil
}

// RevokeUser 清空该用户全部刷新令牌，返回作废数量。
func (s *RefreshStore) RevokeUser(ctx context.Context, userID string) (int, error) {
	key := refreshUserPrefix + userID
	tokens, err := s.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("oidc: 读取用户刷新令牌集合: %w", err)
	}

	pipe := s.rdb.TxPipeline()
	for _, token := range tokens {
		pipe.Del(ctx, refreshPrefix+token)
	}
	pipe.Del(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("oidc: 清空用户刷新令牌: %w", err)
	}
	return len(tokens), nil
}
