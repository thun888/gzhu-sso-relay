package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
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
)

type Upstream struct {
	CASBaseURL    string
	PortalBaseURL string
	Timeout       time.Duration
}

type Service struct {
	users        *repo.UserRepo
	accounts     *repo.AccountRepo
	sessions     *session.Manager
	throttle     *Throttle
	upstream     Upstream
	emailDomains config.EmailDomains
}

type Options struct {
	Users        *repo.UserRepo
	Accounts     *repo.AccountRepo
	Sessions     *session.Manager
	Throttle     *Throttle
	Upstream     Upstream
	EmailDomains config.EmailDomains
}

func NewService(opts Options) *Service {
	return &Service{
		users:        opts.Users,
		accounts:     opts.Accounts,
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
func (s *Service) Login(ctx context.Context, id, password string) (*Result, error) {
	user, err := s.users.ByID(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return s.provision(ctx, id, password)
	}
	if err != nil {
		return nil, err
	}

	if ok, verr := cryptox.VerifyPassword(user.PasswordHash, password); verr == nil && ok {
		return s.issue(ctx, user)
	}

	profile, err := s.authenticateUpstream(ctx, id, password)
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

	if err := s.accounts.SaveProfile(ctx, id, repo.UpstreamProvider, profile.Sub, profile); err != nil {
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

func (s *Service) provision(ctx context.Context, id, password string) (*Result, error) {
	profile, err := s.authenticateUpstream(ctx, id, password)
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
	if err := s.accounts.SaveProfile(ctx, id, repo.UpstreamProvider, profile.Sub, profile); err != nil {
		return nil, err
	}

	return s.issue(ctx, user)
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

func (s *Service) authenticateUpstream(ctx context.Context, id, password string) (portal.Profile, error) {
	allowed, err := s.throttle.Allow(ctx, id)
	if err != nil {
		return portal.Profile{}, err
	}
	if !allowed {
		return portal.Profile{}, ErrTooManyAttempts
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return portal.Profile{}, fmt.Errorf("auth: 创建 cookie jar: %w", err)
	}
	httpClient := &http.Client{Jar: jar, Timeout: s.upstream.Timeout}

	casClient := cas.NewClient(httpClient)
	casClient.BaseURL = s.upstream.CASBaseURL
	if err := casClient.Login(ctx, id, password); err != nil {
		if errors.Is(err, cas.ErrInvalidCredentials) {
			return portal.Profile{}, ErrInvalidCredentials
		}
		return portal.Profile{}, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}

	portalClient := portal.NewClient(httpClient)
	portalClient.BaseURL = s.upstream.PortalBaseURL
	if err := portalClient.Bootstrap(ctx); err != nil {
		return portal.Profile{}, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}

	profile, err := portalClient.Profile(ctx)
	if err != nil {
		return portal.Profile{}, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}
	return profile, nil
}
