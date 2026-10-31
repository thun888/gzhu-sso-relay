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
	codePrefix = "relay:code:"
	codeTTL    = 5 * time.Minute
)

var ErrCodeInvalid = errors.New("oidc: 授权码无效或已使用")

type Grant struct {
	UserID        string   `json:"user_id"`
	ClientID      string   `json:"client_id"`
	RedirectURI   string   `json:"redirect_uri"`
	Scopes        []string `json:"scopes"`
	Nonce         string   `json:"nonce,omitempty"`
	CodeChallenge string   `json:"code_challenge,omitempty"`
	AuthTime      int64    `json:"auth_time"`
}

type CodeStore struct {
	rdb *redis.Client
}

func NewCodeStore(rdb *redis.Client) *CodeStore {
	return &CodeStore{rdb: rdb}
}

func (s *CodeStore) Issue(ctx context.Context, grant Grant) (string, error) {
	raw, err := json.Marshal(grant)
	if err != nil {
		return "", fmt.Errorf("oidc: 序列化授权码: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("oidc: 生成授权码: %w", err)
	}
	code := base64.RawURLEncoding.EncodeToString(buf)

	if err := s.rdb.Set(ctx, codePrefix+code, raw, codeTTL).Err(); err != nil {
		return "", fmt.Errorf("oidc: 写入授权码: %w", err)
	}
	return code, nil
}

// Peek 读取授权码但不删除，供令牌端点先完成客户端与 PKCE 校验。
func (s *CodeStore) Peek(ctx context.Context, code string) (*Grant, error) {
	if code == "" {
		return nil, ErrCodeInvalid
	}

	raw, err := s.rdb.Get(ctx, codePrefix+code).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCodeInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("oidc: 读取授权码: %w", err)
	}

	var grant Grant
	if err := json.Unmarshal(raw, &grant); err != nil {
		return nil, fmt.Errorf("oidc: 解析授权码: %w", err)
	}
	return &grant, nil
}

// Consume 删除授权码并报告是否由本次调用删除，保证只有一个请求能换到令牌。
// 放在校验之后调用，校验失败的请求不会消耗掉别人的授权码。
func (s *CodeStore) Consume(ctx context.Context, code string) (bool, error) {
	if code == "" {
		return false, nil
	}
	n, err := s.rdb.Del(ctx, codePrefix+code).Result()
	if err != nil {
		return false, fmt.Errorf("oidc: 删除授权码: %w", err)
	}
	return n > 0, nil
}
