package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/thun888/gzhu-sso-relay/internal/session"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

const accessTokenTTL = time.Hour

type Provider struct {
	Issuer     string
	LoginURL   string
	ConsentURL string
	AccountURL string

	keys     *Keys
	clients  *repo.ClientRepo
	accounts *repo.AccountRepo
	users    *repo.UserRepo
	sessions *session.Manager
	codes    *CodeStore
	consents *repo.ConsentRepo
	pending  *PendingStore
}

type Options struct {
	Issuer     string
	LoginURL   string
	ConsentURL string
	AccountURL string
	Keys       *Keys
	Clients    *repo.ClientRepo
	Accounts   *repo.AccountRepo
	Users      *repo.UserRepo
	Sessions   *session.Manager
	Codes      *CodeStore
	Consents   *repo.ConsentRepo
	Pending    *PendingStore
}

func New(opts Options) *Provider {
	return &Provider{
		Issuer:     strings.TrimSuffix(opts.Issuer, "/"),
		LoginURL:   opts.LoginURL,
		ConsentURL: opts.ConsentURL,
		AccountURL: opts.AccountURL,
		keys:       opts.Keys,
		clients:    opts.Clients,
		accounts:   opts.Accounts,
		users:      opts.Users,
		sessions:   opts.Sessions,
		codes:      opts.Codes,
		consents:   opts.Consents,
		pending:    opts.Pending,
	}
}

func (p *Provider) sign(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = p.keys.kid
	signed, err := token.SignedString(p.keys.private)
	if err != nil {
		return "", fmt.Errorf("oidc: 签发令牌: %w", err)
	}
	return signed, nil
}

func (p *Provider) verify(raw string) (jwt.MapClaims, error) {
	parsed, err := jwt.Parse(raw,
		func(*jwt.Token) (any, error) { return &p.keys.private.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(p.Issuer),
	)
	if err != nil {
		return nil, fmt.Errorf("oidc: 校验令牌: %w", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("oidc: 令牌 claims 类型不正确")
	}
	return claims, nil
}

// claimsFor 按已授予的 scope 裁剪身份字段，未授予的 scope 不产生任何对应 claim。
func (p *Provider) claimsFor(ctx context.Context, user *model.User, scopes []string) map[string]any {
	claims := map[string]any{"sub": user.ID}

	if hasScope(scopes, "profile") {
		claims["preferred_username"] = user.ID
		claims["name"] = user.Name
		if class := p.classOf(ctx, user.ID); class != "" {
			claims["belong_class"] = class
		}
	}
	if hasScope(scopes, "email") {
		claims["email"] = user.Email
		claims["email_verified"] = true
	}
	return claims
}

func (p *Provider) classOf(ctx context.Context, userID string) string {
	raw, err := p.accounts.ProfileData(ctx, userID, repo.UpstreamProvider)
	if err != nil || len(raw) == 0 {
		return ""
	}
	var snapshot struct {
		Class string `json:"belong_class"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return ""
	}
	return snapshot.Class
}

func (p *Provider) currentUser(c *gin.Context) (*model.User, error) {
	token, _ := c.Cookie(session.CookieName)
	if token == "" {
		if after, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer "); ok {
			token = strings.TrimSpace(after)
		}
	}

	userID, err := p.sessions.Resolve(c.Request.Context(), token)
	if err != nil {
		return nil, err
	}
	user, err := p.users.ByID(c.Request.Context(), userID)
	if err != nil {
		return nil, err
	}
	if user.Status == model.UserStatusDisabled {
		return nil, errors.New("oidc: 用户已被禁用")
	}
	return user, nil
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func splitScope(raw string) []string {
	fields := strings.Fields(raw)
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, s := range fields {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func scopeSubset(requested, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, s := range allowed {
		set[s] = struct{}{}
	}
	for _, s := range requested {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}
