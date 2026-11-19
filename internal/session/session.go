package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
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

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Method    string    `json:"method"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

type Manager struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(rdb *redis.Client) *Manager {
	return &Manager{rdb: rdb, ttl: DefaultTTL}
}

func (m *Manager) Create(ctx context.Context, value Session) (token, id string, err error) {
	token, err = newToken()
	if err != nil {
		return "", "", err
	}
	if value.ID == "" {
		value.ID = uuid.NewString()
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now()
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return "", "", fmt.Errorf("session: 序列化会话: %w", err)
	}

	pipe := m.rdb.TxPipeline()
	pipe.Set(ctx, keyPrefix+token, raw, m.ttl)
	pipe.SAdd(ctx, userPrefix+value.UserID, token)
	pipe.Expire(ctx, userPrefix+value.UserID, m.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", "", fmt.Errorf("session: 写入会话: %w", err)
	}
	return token, value.ID, nil
}

func (m *Manager) Resolve(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, ErrNotFound
	}

	raw, err := m.rdb.Get(ctx, keyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("session: 读取会话: %w", err)
	}

	var value Session
	if json.Unmarshal([]byte(raw), &value) == nil && value.UserID != "" {
		return &value, nil
	}

	// 旧值直接存 userID，读到时就地升级，升级失败也不影响本次鉴权。
	value = Session{ID: uuid.NewString(), UserID: raw, CreatedAt: time.Now()}
	m.upgrade(ctx, token, value)
	return &value, nil
}

func (m *Manager) TTL() time.Duration {
	return m.ttl
}

func (m *Manager) List(ctx context.Context, userID string) ([]Session, error) {
	stored, err := m.list(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]Session, 0, len(stored))
	for _, item := range stored {
		if item.value.ID == "" {
			continue
		}
		out = append(out, item.value)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (m *Manager) Destroy(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	raw, err := m.rdb.Get(ctx, keyPrefix+token).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("session: 读取会话: %w", err)
	}

	var userID string
	if err == nil {
		var value Session
		if json.Unmarshal([]byte(raw), &value) == nil && value.UserID != "" {
			userID = value.UserID
		} else {
			userID = raw
		}
	}

	pipe := m.rdb.TxPipeline()
	pipe.Del(ctx, keyPrefix+token)
	if userID != "" {
		pipe.SRem(ctx, userPrefix+userID, token)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("session: 删除会话: %w", err)
	}
	return nil
}

// RevokeOne 按会话 ID 下线单个设备，返回是否命中。
func (m *Manager) RevokeOne(ctx context.Context, userID, id string) (bool, error) {
	stored, err := m.list(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, item := range stored {
		if item.value.ID != id {
			continue
		}
		pipe := m.rdb.TxPipeline()
		pipe.Del(ctx, keyPrefix+item.token)
		pipe.SRem(ctx, userPrefix+userID, item.token)
		if _, err := pipe.Exec(ctx); err != nil {
			return false, fmt.Errorf("session: 下线会话: %w", err)
		}
		return true, nil
	}
	return false, nil
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

type storedSession struct {
	token string
	value Session
}

func (m *Manager) list(ctx context.Context, userID string) ([]storedSession, error) {
	tokens, err := m.rdb.SMembers(ctx, userPrefix+userID).Result()
	if err != nil {
		return nil, fmt.Errorf("session: 读取用户会话集合: %w", err)
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	keys := make([]string, len(tokens))
	for i, token := range tokens {
		keys[i] = keyPrefix + token
	}
	values, err := m.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("session: 批量读取会话: %w", err)
	}

	out := make([]storedSession, 0, len(tokens))
	for i, value := range values {
		raw, ok := value.(string)
		if !ok {
			continue
		}
		var parsed Session
		if json.Unmarshal([]byte(raw), &parsed) != nil || parsed.UserID == "" {
			continue
		}
		out = append(out, storedSession{token: tokens[i], value: parsed})
	}
	return out, nil
}

func (m *Manager) upgrade(ctx context.Context, token string, value Session) {
	ttl, err := m.rdb.TTL(ctx, keyPrefix+token).Result()
	if err != nil || ttl <= 0 {
		ttl = m.ttl
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = m.rdb.Set(ctx, keyPrefix+token, raw, ttl).Err()
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("session: 生成令牌: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
