package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
	"github.com/thun888/gzhu-sso-relay/internal/upstream/cas"
	"github.com/thun888/gzhu-sso-relay/internal/upstream/portal"
)

const (
	qrPrefix = "relay:qrlogin:"
	// QRTTL 与上游二维码一致：上游按 3 秒一次轮询 60 次后失效。
	QRTTL = 3 * time.Minute
	// qrPollGap 是两次访问上游的最小间隔，比它更快的轮询直接沿用上一次结果。
	qrPollGap = 1500 * time.Millisecond
	// qrConfirmedTTL 让并发的第二次轮询拿到同一个会话，不必重走一遍上游。
	qrConfirmedTTL = time.Minute
	// 轮询锁的兜底 TTL：比上游超时长一截，持锁实例崩溃后靠它自动放出。
	qrLockSlack = 5 * time.Second

	qrLockPrefix  = "relay:qrlock:"
	qrStartPrefix = "relay:qrstart:"
	// 同一 IP 每分钟最多生成 12 个二维码，正常用户点不到这个量。
	qrStartLimit  = 12
	qrStartWindow = time.Minute
)

// qrUnlockScript 只删除自己持有的锁，避免误删后一个轮询刚设置的锁。
var qrUnlockScript = redis.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`)

var (
	ErrQRSessionInvalid = errors.New("auth: 二维码已失效")
	ErrQRUnlinked       = errors.New("auth: 该微信未关联统一身份认证账号")
	ErrQRBindMismatch   = errors.New("auth: 轮询请求与发起二维码的浏览器不一致")
)

// QRSession 保存一轮扫码登录的中间状态。上游 cookie 一并落库，
// 轮询打到别的实例时也能续上同一份会话。
type QRSession struct {
	UUID      string `json:"uuid"`
	UserAgent string `json:"user_agent"`
	// BindHash 是发起浏览器绑定 cookie 的 SHA-256，轮询必须持同一份 cookie 才认。
	BindHash string         `json:"bind_hash"`
	Cookies  []*http.Cookie `json:"cookies"`
	LastPoll int64          `json:"last_poll"`
	UserID   string         `json:"user_id"`
	Token    string         `json:"token"`
}

type QRStore struct {
	rdb *redis.Client
}

func NewQRStore(rdb *redis.Client) *QRStore {
	return &QRStore{rdb: rdb}
}

func (s *QRStore) save(ctx context.Context, id string, session QRSession, ttl time.Duration) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("auth: 序列化扫码会话: %w", err)
	}
	if err := s.rdb.Set(ctx, qrPrefix+id, payload, ttl).Err(); err != nil {
		return fmt.Errorf("auth: 保存扫码会话: %w", err)
	}
	return nil
}

func (s *QRStore) load(ctx context.Context, id string) (QRSession, error) {
	var session QRSession
	if id == "" {
		return session, ErrQRSessionInvalid
	}

	raw, err := s.rdb.Get(ctx, qrPrefix+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return session, ErrQRSessionInvalid
	}
	if err != nil {
		return session, fmt.Errorf("auth: 读取扫码会话: %w", err)
	}
	if err := json.Unmarshal(raw, &session); err != nil {
		return session, fmt.Errorf("auth: 解析扫码会话: %w", err)
	}
	return session, nil
}

func (s *QRStore) delete(ctx context.Context, id string) error {
	return s.rdb.Del(ctx, qrPrefix+id).Err()
}

// allowStart 按固定窗口限制同一 IP 生成二维码的频率。
func (s *QRStore) allowStart(ctx context.Context, ip string) (bool, error) {
	if ip == "" {
		return true, nil
	}
	key := qrStartPrefix + ip
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("auth: 扫码生成计数: %w", err)
	}
	if count == 1 {
		if err := s.rdb.Expire(ctx, key, qrStartWindow).Err(); err != nil {
			return false, fmt.Errorf("auth: 扫码生成计数过期: %w", err)
		}
	}
	return count <= qrStartLimit, nil
}

// lock 给一次轮询的变更路径加互斥，拿不到锁说明已有实例在处理，直接当作 pending。
func (s *QRStore) lock(ctx context.Context, id string, ttl time.Duration) (func(), bool, error) {
	token := uuid.NewString()
	ok, err := s.rdb.SetNX(ctx, qrLockPrefix+id, token, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("auth: 扫码轮询加锁: %w", err)
	}
	if !ok {
		return nil, false, nil
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = qrUnlockScript.Run(ctx, s.rdb, []string{qrLockPrefix + id}, token).Err()
	}, true, nil
}

// QRChallenge 是一次扫码登录的二维码，ID 既是轮询凭据也是上游绑定用的 uuid。
type QRChallenge struct {
	ID        string
	URL       string
	ExpiresIn time.Duration
}

func (s *Service) StartQRLogin(ctx context.Context, ip, userAgent, bindHash string) (*QRChallenge, error) {
	ok, err := s.qr.allowStart(ctx, ip)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTooManyAttempts
	}

	id := uuid.NewString()
	session := QRSession{UUID: id, UserAgent: userAgent, BindHash: bindHash}
	if err := s.qr.save(ctx, id, session, QRTTL); err != nil {
		return nil, err
	}
	return &QRChallenge{
		ID:        id,
		URL:       cas.QRLoginURL(s.upstream.CASBaseURL, id, s.upstream.QRService),
		ExpiresIn: QRTTL,
	}, nil
}

// PollQRLogin 查询一次扫码状态，返回 nil 表示仍在等待扫码。
// bindHash 是轮询请求携带的绑定 cookie 哈希，与发起时不一致直接拒绝。
func (s *Service) PollQRLogin(ctx context.Context, id, bindHash string) (*Result, error) {
	session, err := s.qr.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if session.BindHash == "" || bindHash == "" ||
		subtle.ConstantTimeCompare([]byte(session.BindHash), []byte(bindHash)) != 1 {
		return nil, ErrQRBindMismatch
	}

	if session.Token != "" {
		user, err := s.users.ByID(ctx, session.UserID)
		if err != nil {
			return nil, ErrQRSessionInvalid
		}
		return &Result{User: user, Token: session.Token}, nil
	}

	release, locked, err := s.qr.lock(ctx, id, s.upstream.Timeout+qrLockSlack)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, nil
	}
	defer release()

	if time.Since(time.Unix(session.LastPoll, 0)) < qrPollGap {
		return nil, nil
	}
	session.LastPoll = time.Now().Unix()

	jar := newCookieJar(session.Cookies)
	hc := &http.Client{Jar: jar, Timeout: s.upstream.Timeout}

	casClient := cas.NewClient(hc)
	casClient.BaseURL = s.upstream.CASBaseURL
	casClient.UserAgent = session.UserAgent

	redirect, err := casClient.QRScan(ctx, session.UUID)
	session.Cookies = jar.snapshot()
	if err != nil {
		_ = s.qr.save(ctx, id, session, QRTTL)
		return nil, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}
	if redirect == "" {
		if err := s.qr.save(ctx, id, session, QRTTL); err != nil {
			return nil, err
		}
		return nil, nil
	}

	result, err := s.confirmQR(ctx, &session, hc, redirect)
	if err != nil {
		_ = s.qr.delete(ctx, id)
		return nil, err
	}

	// 确认后的会话只留本地令牌，上游 cookie 用完即弃，不落 Redis。
	session.Cookies = nil
	session.UserID = result.User.ID
	session.Token = result.Token
	if err := s.qr.save(ctx, id, session, qrConfirmedTTL); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) confirmQR(ctx context.Context, session *QRSession, hc *http.Client, redirect string) (*Result, error) {
	// redirect 里带着上游票据，跟进一次才能把扫码建立的会话换成门户自己的 cookie。
	if err := followRedirect(ctx, hc, redirect); err != nil {
		log.Printf("扫码登录：跟进上游跳转失败 uuid=%s: %v", session.UUID, err)
	}

	portalClient := portal.NewClient(hc)
	portalClient.BaseURL = s.upstream.PortalBaseURL
	portalClient.UserAgent = session.UserAgent

	if err := portalClient.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}
	profile, err := portalClient.Profile(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}

	user, err := s.qrUser(ctx, hc, session.UserAgent, profile)
	if err != nil {
		return nil, err
	}
	return s.issue(ctx, user)
}

// qrUser 先按门户返回的手机号找已绑定账号。门户不含学号，首次扫码还要向上游反查，
// 查不到就让用户先用账密登录一次。
func (s *Service) qrUser(ctx context.Context, hc *http.Client, userAgent string, profile portal.Profile) (*model.User, error) {
	account, err := s.accounts.ByProviderUserID(ctx, repo.UpstreamProvider, profile.Sub)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return nil, err
	}
	if err == nil {
		user, err := s.users.ByID(ctx, account.UserID)
		switch {
		case err == nil:
			if err := s.saveUpstreamProfile(ctx, user.ID, profile); err != nil {
				return nil, err
			}
			return user, nil
		case errors.Is(err, repo.ErrNotFound):
		default:
			return nil, err
		}
	}

	id, err := s.upstreamAccountID(ctx, hc, userAgent)
	if err != nil {
		return nil, err
	}

	user, err := s.users.ByID(ctx, id)
	switch {
	case err == nil:
		if err := s.saveUpstreamProfile(ctx, id, profile); err != nil {
			return nil, err
		}
		return user, nil
	case errors.Is(err, repo.ErrNotFound):
	default:
		return nil, err
	}

	suffix, ok := s.emailDomains.Suffix(profile.Sub)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoEmailDomain, profile.Sub)
	}

	user = &model.User{
		ID:     id,
		Name:   profile.Name,
		Email:  profile.Sub + "@" + suffix,
		Type:   model.UserTypeNormal,
		Status: model.UserStatusActive,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	if err := s.saveUpstreamProfile(ctx, id, profile); err != nil {
		return nil, err
	}
	return user, nil
}

// upstreamAccountID 用扫码建立的 SSO 会话申请一张票据，从统一认证反查学号或教工号。
func (s *Service) upstreamAccountID(ctx context.Context, hc *http.Client, userAgent string) (string, error) {
	portalClient := portal.NewClient(hc)
	portalClient.BaseURL = s.upstream.PortalBaseURL
	portalClient.UserAgent = userAgent

	service, err := portalClient.ServiceURL(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrQRUnlinked, err)
	}

	casClient := cas.NewClient(hc)
	casClient.BaseURL = s.upstream.CASBaseURL
	casClient.UserAgent = userAgent

	ticket, err := casClient.ServiceTicket(ctx, service)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrQRUnlinked, err)
	}
	principal, err := casClient.Principal(ctx, service, ticket)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrQRUnlinked, err)
	}
	if !isAccountID(principal) {
		return "", fmt.Errorf("%w：上游账号 %q 不是学号或教工号", ErrQRUnlinked, principal)
	}
	return principal, nil
}

func followRedirect(ctx context.Context, hc *http.Client, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return nil
}

func isAccountID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
