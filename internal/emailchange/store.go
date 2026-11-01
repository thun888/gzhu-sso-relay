package emailchange

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	tokenPrefix = "relay:emailchg:token:"
	userPrefix  = "relay:emailchg:user:"
)

var ErrInvalid = errors.New("emailchange: 确认链接无效或已过期")

type Request struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	OldEmail string `json:"old_email"`
}

type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb, ttl: 30 * time.Minute}
}

func (s *Store) TTL() time.Duration { return s.ttl }

// Issue 使同一用户先前的待确认请求失效，再签发新 token。
func (s *Store) Issue(ctx context.Context, req Request) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("emailchange: 生成 token: %w", err)
	}
	token := hex.EncodeToString(buf)

	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("emailchange: 序列化请求: %w", err)
	}

	userKey := userPrefix + req.UserID
	old, _ := s.rdb.Get(ctx, userKey).Result()
	pipe := s.rdb.TxPipeline()
	if old != "" {
		pipe.Del(ctx, tokenPrefix+old)
	}
	pipe.Set(ctx, tokenPrefix+token, payload, s.ttl)
	pipe.Set(ctx, userKey, token, s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("emailchange: 写入待确认请求: %w", err)
	}
	return token, nil
}

func (s *Store) Consume(ctx context.Context, token string) (*Request, error) {
	if token == "" {
		return nil, ErrInvalid
	}
	raw, err := s.rdb.GetDel(ctx, tokenPrefix+token).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("emailchange: 读取待确认请求: %w", err)
	}

	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("emailchange: 解析待确认请求: %w", err)
	}
	s.rdb.Del(ctx, userPrefix+req.UserID)
	return &req, nil
}
