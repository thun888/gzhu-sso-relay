package passkey

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/redis/go-redis/v9"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

const (
	registerPrefix = "relay:passkey:reg:"
	loginPrefix    = "relay:passkey:login:"
	CeremonyTTL    = 5 * time.Minute
	defaultName    = "未命名设备"
	maxNameLength  = 40
)

var (
	ErrCeremony          = errors.New("passkey: 验证会话不存在或已过期")
	ErrUnknownCredential = errors.New("passkey: 该通行密钥未在本站登记")
)

type Options struct {
	Users       *repo.UserRepo
	Passkeys    *repo.PasskeyRepo
	RDB         *redis.Client
	BaseURL     string
	DisplayName string
}

// Service 负责通行密钥的注册与登录仪式，挑战暂存在 Redis 并按浏览器的 cookie 取用。
type Service struct {
	wa       *webauthn.WebAuthn
	users    *repo.UserRepo
	passkeys *repo.PasskeyRepo
	rdb      *redis.Client
}

func New(opts Options) (*Service, error) {
	rpID, origin, err := relyingParty(opts.BaseURL)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(opts.DisplayName)
	if name == "" {
		name = "统一身份认证"
	}

	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: name,
		RPOrigins:     []string{origin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationPreferred,
		},
		AttestationPreference: protocol.PreferNoAttestation,
	})
	if err != nil {
		return nil, fmt.Errorf("passkey: 初始化 WebAuthn: %w", err)
	}

	return &Service{wa: wa, users: opts.Users, passkeys: opts.Passkeys, rdb: opts.RDB}, nil
}

func (s *Service) Enabled() bool {
	return s != nil && s.wa != nil
}

func (s *Service) List(ctx context.Context, userID string) ([]model.Passkey, error) {
	return s.passkeys.ListByUser(ctx, userID)
}

func (s *Service) Delete(ctx context.Context, userID, id string) (bool, error) {
	return s.passkeys.Delete(ctx, userID, id)
}

// relyingParty 从 base_url 推导 RP ID 与来源，两者必须对应站点实际访问的域。
func relyingParty(baseURL string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSuffix(baseURL, "/"))
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return "", "", fmt.Errorf("passkey: base_url 无法解析为站点地址: %s", baseURL)
	}
	return parsed.Hostname(), parsed.Scheme + "://" + parsed.Host, nil
}

func NewSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("passkey: 生成验证凭据: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

type ceremony struct {
	UserID  string               `json:"user_id"`
	Name    string               `json:"name,omitempty"`
	Session webauthn.SessionData `json:"session"`
}

func (s *Service) save(ctx context.Context, prefix, secret string, data ceremony) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("passkey: 序列化验证会话: %w", err)
	}
	if err := s.rdb.Set(ctx, prefix+secret, payload, CeremonyTTL).Err(); err != nil {
		return fmt.Errorf("passkey: 暂存验证会话: %w", err)
	}
	return nil
}

func (s *Service) take(ctx context.Context, prefix, secret string) (*ceremony, error) {
	if secret == "" {
		return nil, ErrCeremony
	}
	raw, err := s.rdb.GetDel(ctx, prefix+secret).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCeremony
	}
	if err != nil {
		return nil, fmt.Errorf("passkey: 读取验证会话: %w", err)
	}

	var data ceremony
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("passkey: 解析验证会话: %w", err)
	}
	return &data, nil
}

type authUser struct {
	user        *model.User
	credentials []webauthn.Credential
}

func (u authUser) WebAuthnID() []byte                         { return []byte(u.user.ID) }
func (u authUser) WebAuthnName() string                       { return u.user.ID }
func (u authUser) WebAuthnDisplayName() string                { return u.user.Name }
func (u authUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *Service) loadUser(ctx context.Context, userID string) (authUser, error) {
	user, err := s.users.ByID(ctx, userID)
	if err != nil {
		return authUser{}, err
	}
	stored, err := s.passkeys.ListByUser(ctx, userID)
	if err != nil {
		return authUser{}, err
	}
	credentials := make([]webauthn.Credential, 0, len(stored))
	for _, item := range stored {
		credential, err := decode(item.Credential)
		if err != nil {
			return authUser{}, err
		}
		credentials = append(credentials, credential)
	}
	return authUser{user: user, credentials: credentials}, nil
}

func (s *Service) BeginRegistration(ctx context.Context, userID, secret, name string) (*protocol.CredentialCreation, error) {
	holder, err := s.loadUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 已绑定的凭据排进排除列表，避免同一台设备重复注册后在写库时撞唯一索引。
	exclusions := make([]protocol.CredentialDescriptor, 0, len(holder.credentials))
	for _, credential := range holder.credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}

	creation, session, err := s.wa.BeginRegistration(holder, webauthn.WithExclusions(exclusions))
	if err != nil {
		return nil, fmt.Errorf("passkey: 生成注册参数: %w", err)
	}
	if err := s.save(ctx, registerPrefix, secret, ceremony{UserID: userID, Name: cleanName(name), Session: *session}); err != nil {
		return nil, err
	}
	return creation, nil
}

func (s *Service) FinishRegistration(ctx context.Context, userID, secret string, r *http.Request) (*model.Passkey, error) {
	pending, err := s.take(ctx, registerPrefix, secret)
	if err != nil {
		return nil, err
	}
	if pending.UserID != userID {
		return nil, ErrCeremony
	}

	holder, err := s.loadUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	credential, err := s.wa.FinishRegistration(holder, pending.Session, r)
	if err != nil {
		return nil, fmt.Errorf("passkey: 校验注册结果: %w", err)
	}

	encoded, err := encode(credential)
	if err != nil {
		return nil, err
	}

	item := &model.Passkey{
		UserID:         userID,
		CredentialID:   base64.RawURLEncoding.EncodeToString(credential.ID),
		Name:           pending.Name,
		Credential:     encoded,
		SignCount:      credential.Authenticator.SignCount,
		BackupEligible: credential.Flags.BackupEligible,
	}
	if err := s.passkeys.Create(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) BeginLogin(ctx context.Context, secret string) (*protocol.CredentialAssertion, error) {
	assertion, session, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, fmt.Errorf("passkey: 生成登录参数: %w", err)
	}
	if err := s.save(ctx, loginPrefix, secret, ceremony{Session: *session}); err != nil {
		return nil, err
	}
	return assertion, nil
}

// FinishLogin 返回完成断言的用户；凭据由浏览器自行挑选，因此用户身份要在校验通过后才确定。
func (s *Service) FinishLogin(ctx context.Context, secret string, r *http.Request) (string, error) {
	pending, err := s.take(ctx, loginPrefix, secret)
	if err != nil {
		return "", err
	}

	var matched *model.Passkey
	var holder authUser

	lookup := func(rawID, _ []byte) (webauthn.User, error) {
		item, err := s.passkeys.ByCredentialID(ctx, base64.RawURLEncoding.EncodeToString(rawID))
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrUnknownCredential
		}
		if err != nil {
			return nil, err
		}
		loaded, err := s.loadUser(ctx, item.UserID)
		if err != nil {
			return nil, err
		}
		matched = item
		holder = loaded
		return holder, nil
	}

	credential, err := s.wa.FinishDiscoverableLogin(lookup, pending.Session, r)
	if err != nil {
		return "", fmt.Errorf("passkey: 校验登录结果: %w", err)
	}
	if matched == nil {
		return "", ErrUnknownCredential
	}

	if credential.Authenticator.CloneWarning {
		log.Printf("通行密钥计数器回退，可能被克隆 user=%s passkey=%s", matched.UserID, matched.ID)
	}

	encoded, err := encode(credential)
	if err != nil {
		return "", err
	}
	if err := s.passkeys.Touch(ctx, matched.ID, encoded, credential.Authenticator.SignCount, time.Now()); err != nil {
		return "", err
	}
	return matched.UserID, nil
}

func encode(credential *webauthn.Credential) (string, error) {
	raw, err := json.Marshal(credential)
	if err != nil {
		return "", fmt.Errorf("passkey: 序列化凭据: %w", err)
	}
	return string(raw), nil
}

func decode(stored string) (webauthn.Credential, error) {
	var credential webauthn.Credential
	if err := json.Unmarshal([]byte(stored), &credential); err != nil {
		return webauthn.Credential{}, fmt.Errorf("passkey: 解析凭据: %w", err)
	}
	return credential, nil
}

func cleanName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return defaultName
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		name = string([]rune(name)[:maxNameLength])
	}
	return name
}
