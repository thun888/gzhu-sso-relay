package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/thun888/gzhu-sso-relay/internal/config"
	"github.com/thun888/gzhu-sso-relay/internal/cryptox"
	"github.com/thun888/gzhu-sso-relay/internal/session"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
	"github.com/thun888/gzhu-sso-relay/internal/upstream/cas"
	"github.com/thun888/gzhu-sso-relay/internal/upstream/portal"
)

var (
	ErrInvalidCredentials  = errors.New("auth: 账号或密码错误")
	ErrNoEmailDomain       = errors.New("auth: 该学号没有匹配的邮箱域名前缀")
	ErrTooManyAttempts     = errors.New("auth: 尝试次数过多，请稍后再试")
	ErrUpstreamUnavailable = errors.New("auth: 上游认证服务不可用")
	ErrUpstreamChallenge   = errors.New("auth: 上游要求验证码，无法自动登录")
	ErrUpstreamLocked      = errors.New("auth: 上游账号已被临时锁定")
)

type Upstream struct {
	CASBaseURL    string
	PortalBaseURL string
	Timeout       time.Duration
	RefreshRate   float64
}

type Service struct {
	users        *repo.UserRepo
	accounts     *repo.AccountRepo
	groups       *repo.GroupRepo
	sessions     *session.Manager
	throttle     *Throttle
	upstream     Upstream
	emailDomains config.EmailDomains
}

type Options struct {
	Users        *repo.UserRepo
	Accounts     *repo.AccountRepo
	Groups       *repo.GroupRepo
	Sessions     *session.Manager
	Throttle     *Throttle
	Upstream     Upstream
	EmailDomains config.EmailDomains
}

func NewService(opts Options) *Service {
	return &Service{
		users:        opts.Users,
		accounts:     opts.Accounts,
		groups:       opts.Groups,
		sessions:     opts.Sessions,
		throttle:     opts.Throttle,
		upstream:     opts.Upstream,
		emailDomains: opts.EmailDomains,
	}
}

type Result struct {
	User  *model.User
	Token string
}

// Login 优先用本地哈希校验；本地不匹配时回访上游，覆盖上游改密与首次登录两种情况。
func (s *Service) Login(ctx context.Context, id, password, userAgent string) (*Result, error) {
	user, err := s.users.ByID(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return s.provision(ctx, id, password, userAgent)
	}
	if err != nil {
		return nil, err
	}

	if ok, verr := cryptox.VerifyPassword(user.PasswordHash, password); verr == nil && ok {
		if user.UpstreamStale {
			return s.reverifyUpstream(ctx, user, password, userAgent)
		}
		result, err := s.issue(ctx, user)
		if err == nil && s.shouldRefresh() {
			s.refreshUpstream(user.ID, password, userAgent)
		}
		return result, err
	}

	profile, err := s.authenticateUpstream(ctx, id, password, userAgent)
	if err != nil {
		return nil, err
	}

	hash, err := cryptox.HashPassword(password)
	if err != nil {
		return nil, err
	}
	if err := s.users.UpdatePasswordHash(ctx, id, hash); err != nil {
		return nil, err
	}
	user.PasswordHash = hash

	if err := s.saveUpstreamProfile(ctx, id, profile); err != nil {
		return nil, err
	}

	return s.issue(ctx, user)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.sessions.Destroy(ctx, token)
}

func (s *Service) SessionTTL() time.Duration {
	return s.sessions.TTL()
}

func (s *Service) Resolve(ctx context.Context, token string) (*model.User, error) {
	id, err := s.sessions.Resolve(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.users.ByID(ctx, id)
}

func (s *Service) provision(ctx context.Context, id, password, userAgent string) (*Result, error) {
	profile, err := s.authenticateUpstream(ctx, id, password, userAgent)
	if err != nil {
		return nil, err
	}

	hash, err := cryptox.HashPassword(password)
	if err != nil {
		return nil, err
	}

	suffix, ok := s.emailDomains.Suffix(profile.Sub)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoEmailDomain, profile.Sub)
	}

	user := &model.User{
		ID:           id,
		Name:         profile.Name,
		PasswordHash: hash,
		Email:        profile.Sub + "@" + suffix,
		Type:         model.UserTypeNormal,
		Status:       model.UserStatusActive,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	if err := s.saveUpstreamProfile(ctx, id, profile); err != nil {
		return nil, err
	}

	return s.issue(ctx, user)
}

// reverifyUpstream 处理本地命中但上次上游验证被拒的登录：同步复核通过才放行，
// 复核失败说明本地哈希已过期，保留标记等待下次复核。
func (s *Service) reverifyUpstream(ctx context.Context, user *model.User, password, userAgent string) (*Result, error) {
	profile, err := s.authenticateUpstream(ctx, user.ID, password, userAgent)
	if err != nil {
		return nil, err
	}
	if err := s.users.SetUpstreamStale(ctx, user.ID, false); err != nil {
		return nil, err
	}
	if err := s.saveUpstreamProfile(ctx, user.ID, profile); err != nil {
		return nil, err
	}
	return s.issue(ctx, user)
}

// shouldRefresh 决定这次登录是否顺带刷新上游资料。本地哈希能命中就不会访问上游，
// 完全靠它同步会让学院、身份这类信息一直停在首次登录时的值。
func (s *Service) shouldRefresh() bool {
	return s.upstream.RefreshRate > 0 && rand.Float64() < s.upstream.RefreshRate
}

// refreshUpstream 在响应返回后再访问上游，不影响这次登录；上游明确拒绝时标记本地密码过期。
func (s *Service) refreshUpstream(userID, password, userAgent string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.upstream.Timeout)
		defer cancel()

		profile, err := s.fetchUpstream(ctx, userID, password, userAgent)
		if err != nil {
			if errors.Is(err, ErrInvalidCredentials) {
				if markErr := s.users.SetUpstreamStale(ctx, userID, true); markErr != nil {
					log.Printf("标记本地密码过期失败 id=%s: %v", userID, markErr)
				}
				return
			}
			log.Printf("刷新上游资料失败 id=%s: %v", userID, err)
			return
		}
		if err := s.saveUpstreamProfile(ctx, userID, profile); err != nil {
			log.Printf("刷新上游资料失败 id=%s: %v", userID, err)
			return
		}
		_ = s.users.SetUpstreamStale(ctx, userID, false)
	}()
}

// saveUpstreamProfile 落库上游快照并重建同步组，只在真正访问过上游后调用。
func (s *Service) saveUpstreamProfile(ctx context.Context, userID string, profile portal.Profile) error {
	if err := s.accounts.SaveProfile(ctx, userID, repo.UpstreamProvider, profile.Sub, profile); err != nil {
		return err
	}
	return s.groups.SyncUpstream(ctx, userID, upstreamGroups(profile))
}

func upstreamGroups(profile portal.Profile) []repo.GroupSpec {
	specs := make([]repo.GroupSpec, 0, 3)
	for _, item := range []struct {
		prefix string
		value  string
	}{
		{"user_type", profile.UserType},
		{"unit", profile.Unit},
		{"major", profile.Major},
	} {
		value := strings.TrimSpace(item.value)
		if value == "" {
			continue
		}
		specs = append(specs, repo.GroupSpec{Code: item.prefix + ":" + value, Name: value})
	}
	return specs
}

func (s *Service) issue(ctx context.Context, user *model.User) (*Result, error) {
	if user.Status == model.UserStatusDisabled {
		return nil, ErrInvalidCredentials
	}

	token, err := s.sessions.Create(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if err := s.users.TouchLogin(ctx, user.ID, now); err != nil {
		return nil, err
	}
	user.LastLoginAt = &now

	_ = s.throttle.Reset(ctx, user.ID)

	return &Result{User: user, Token: token}, nil
}

func (s *Service) authenticateUpstream(ctx context.Context, id, password, userAgent string) (portal.Profile, error) {
	allowed, err := s.throttle.Allow(ctx, id)
	if err != nil {
		return portal.Profile{}, err
	}
	if !allowed {
		return portal.Profile{}, ErrTooManyAttempts
	}
	return s.fetchUpstream(ctx, id, password, userAgent)
}

func (s *Service) fetchUpstream(ctx context.Context, id, password, userAgent string) (portal.Profile, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return portal.Profile{}, fmt.Errorf("auth: 创建 cookie jar: %w", err)
	}
	httpClient := &http.Client{Jar: jar, Timeout: s.upstream.Timeout}

	casClient := cas.NewClient(httpClient)
	casClient.BaseURL = s.upstream.CASBaseURL
	casClient.UserAgent = userAgent
	if err := casClient.Login(ctx, id, password); err != nil {
		switch {
		case errors.Is(err, cas.ErrInvalidCredentials):
			return portal.Profile{}, ErrInvalidCredentials
		case errors.Is(err, cas.ErrChallengeRequired):
			return portal.Profile{}, fmt.Errorf("%w: %v", ErrUpstreamChallenge, err)
		case errors.Is(err, cas.ErrAccountLocked):
			return portal.Profile{}, fmt.Errorf("%w: %v", ErrUpstreamLocked, err)
		}
		return portal.Profile{}, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}

	portalClient := portal.NewClient(httpClient)
	portalClient.BaseURL = s.upstream.PortalBaseURL
	portalClient.UserAgent = userAgent
	if err := portalClient.Bootstrap(ctx); err != nil {
		return portal.Profile{}, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}

	profile, err := portalClient.Profile(ctx)
	if err != nil {
		return portal.Profile{}, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}
	return profile, nil
}
