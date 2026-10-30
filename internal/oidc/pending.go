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
	pendingPrefix = "relay:consent:"
	pendingTTL    = 10 * time.Minute
)

var ErrPendingInvalid = errors.New("oidc: 授权请求不存在或已过期")

// Pending 暂存一次待确认的授权请求，确认页只凭不透明 id 取用，避免被伪造参数。
type Pending struct {
	UserID       string   `json:"user_id"`
	ClientID     string   `json:"client_id"`
	Scopes       []string `json:"scopes"`
	AuthorizeURL string   `json:"authorize_url"`
}

type PendingStore struct {
	rdb *redis.Client
}

func NewPendingStore(rdb *redis.Client) *PendingStore {
	return &PendingStore{rdb: rdb}
}

func (s *PendingStore) Save(ctx context.Context, pending Pending) (string, error) {
	payload, err := json.Marshal(pending)
	if err != nil {
		return "", err
	}

	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("oidc: 生成授权请求 id: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(buf)

	if err := s.rdb.Set(ctx, pendingPrefix+id, payload, pendingTTL).Err(); err != nil {
		return "", fmt.Errorf("oidc: 保存授权请求: %w", err)
	}
	return id, nil
}

func (s *PendingStore) Load(ctx context.Context, id string) (*Pending, error) {
	if id == "" {
		return nil, ErrPendingInvalid
	}

	raw, err := s.rdb.Get(ctx, pendingPrefix+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrPendingInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("oidc: 读取授权请求: %w", err)
	}

	var pending Pending
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, fmt.Errorf("oidc: 解析授权请求: %w", err)
	}
	return &pending, nil
}

func (s *PendingStore) Delete(ctx context.Context, id string) error {
	return s.rdb.Del(ctx, pendingPrefix+id).Err()
}
