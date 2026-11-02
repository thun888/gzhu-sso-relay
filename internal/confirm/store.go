package confirm

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

// Kind 区分同一套确认机制的用途，令牌与待确认请求按用途隔离。
type Kind string

const (
	EmailChange   Kind = "email"
	AccountDelete Kind = "account"
)

var ErrInvalid = errors.New("confirm: 确认链接无效或已过期")

type entry struct {
	UserID string          `json:"user_id"`
	Data   json.RawMessage `json:"data"`
}

type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb, ttl: 30 * time.Minute}
}

func (s *Store) TTL() time.Duration { return s.ttl }

func tokenKey(kind Kind, token string) string {
	return "relay:confirm:" + string(kind) + ":token:" + token
}

func userKey(kind Kind, userID string) string {
	return "relay:confirm:" + string(kind) + ":user:" + userID
}

// Issue 使同一用户同一用途下先前的待确认请求失效，再签发新 token。
func (s *Store) Issue(ctx context.Context, kind Kind, userID string, data any) (string, error) {
	var payload json.RawMessage
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return "", fmt.Errorf("confirm: 序列化请求: %w", err)
		}
		payload = raw
	}

	encoded, err := json.Marshal(entry{UserID: userID, Data: payload})
	if err != nil {
		return "", fmt.Errorf("confirm: 序列化请求: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("confirm: 生成 token: %w", err)
	}
	token := hex.EncodeToString(buf)

	pending := userKey(kind, userID)
	old, _ := s.rdb.Get(ctx, pending).Result()
	pipe := s.rdb.TxPipeline()
	if old != "" {
		pipe.Del(ctx, tokenKey(kind, old))
	}
	pipe.Set(ctx, tokenKey(kind, token), encoded, s.ttl)
	pipe.Set(ctx, pending, token, s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("confirm: 写入待确认请求: %w", err)
	}
	return token, nil
}

func (s *Store) Consume(ctx context.Context, kind Kind, token string) (string, json.RawMessage, error) {
	if token == "" {
		return "", nil, ErrInvalid
	}

	raw, err := s.rdb.GetDel(ctx, tokenKey(kind, token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return "", nil, ErrInvalid
	}
	if err != nil {
		return "", nil, fmt.Errorf("confirm: 读取待确认请求: %w", err)
	}

	var req entry
	if err := json.Unmarshal(raw, &req); err != nil {
		return "", nil, fmt.Errorf("confirm: 解析待确认请求: %w", err)
	}
	s.rdb.Del(ctx, userKey(kind, req.UserID))
	return req.UserID, req.Data, nil
}

// Discard 丢弃该用户某个用途下的待确认请求，用于流程作废或账号数据清理。
func (s *Store) Discard(ctx context.Context, kind Kind, userID string) error {
	pending := userKey(kind, userID)
	token, err := s.rdb.Get(ctx, pending).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("confirm: 读取待确认请求: %w", err)
	}

	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, tokenKey(kind, token))
	pipe.Del(ctx, pending)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("confirm: 丢弃待确认请求: %w", err)
	}
	return nil
}
