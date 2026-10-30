package binding

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

const statePrefix = "relay:bindstate:"

var ErrStateInvalid = errors.New("binding: 绑定状态无效或已过期")

type State struct {
	Provider string `json:"provider"`
	UserID   string `json:"user_id"`
	Next     string `json:"next"`
}

type StateStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewStateStore(rdb *redis.Client) *StateStore {
	return &StateStore{rdb: rdb, ttl: 10 * time.Minute}
}

func (s *StateStore) Issue(ctx context.Context, state State) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}

	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("binding: 生成 state: %w", err)
	}
	key := base64.RawURLEncoding.EncodeToString(buf)

	if err := s.rdb.Set(ctx, statePrefix+key, payload, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("binding: 写入 state: %w", err)
	}
	return key, nil
}

func (s *StateStore) Consume(ctx context.Context, key string) (*State, error) {
	if key == "" {
		return nil, ErrStateInvalid
	}

	raw, err := s.rdb.GetDel(ctx, statePrefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrStateInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("binding: 读取 state: %w", err)
	}

	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("binding: 解析 state: %w", err)
	}
	return &state, nil
}
