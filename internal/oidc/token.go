package oidc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/thun888/gzhu-sso-relay/internal/cryptox"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

func (p *Provider) Token(c *gin.Context) {
	ctx := c.Request.Context()
	if err := c.Request.ParseForm(); err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_request", "表单无法解析")
		return
	}
	form := c.Request.PostForm

	if p.limiter != nil {
		blocked, err := p.limiter.Blocked(ctx, c.ClientIP())
		if err != nil {
			tokenError(c, http.StatusInternalServerError, "server_error", "限流状态不可用")
			return
		}
		if blocked {
			tokenError(c, http.StatusTooManyRequests, "too_many_requests", "失败次数过多，请稍后再试")
			return
		}
	}

	clientID, secret, ok := clientCredentials(c, form)
	if !ok || clientID == "" {
		p.countFailure(ctx, c)
		tokenError(c, http.StatusUnauthorized, "invalid_client", "缺少客户端凭据")
		return
	}

	client, err := p.clients.ByClientID(ctx, clientID)
	if err != nil || !client.Enabled {
		dummyVerify(secret)
		p.countFailure(ctx, c)
		tokenError(c, http.StatusUnauthorized, "invalid_client", "未知的客户端")
		return
	}
	if !client.IsPublic && !secretMatches(client, secret) {
		p.countFailure(ctx, c)
		tokenError(c, http.StatusUnauthorized, "invalid_client", "客户端密钥不正确")
		return
	}

	switch form.Get("grant_type") {
	case "authorization_code":
		p.exchangeCode(c, client, form)
	case "refresh_token":
		p.exchangeRefresh(c, client, form)
	default:
		tokenError(c, http.StatusBadRequest, "unsupported_grant_type", "仅支持 authorization_code 与 refresh_token")
	}
}

func (p *Provider) exchangeCode(c *gin.Context, client *model.OAuthClient, form url.Values) {
	ctx := c.Request.Context()

	grant, err := p.codes.Peek(ctx, form.Get("code"))
	if err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "授权码无效或已过期")
		return
	}
	if grant.ClientID != client.ClientID {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "授权码与客户端不匹配")
		return
	}
	if got := form.Get("redirect_uri"); got != grant.RedirectURI {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "redirect_uri 不匹配")
		return
	}
	if client.IsPublic && grant.CodeChallenge == "" {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "公开客户端必须使用 PKCE")
		return
	}
	if !verifyPKCE(grant, form.Get("code_verifier")) {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "PKCE 校验失败")
		return
	}

	consumed, err := p.codes.Consume(ctx, form.Get("code"))
	if err != nil {
		tokenError(c, http.StatusInternalServerError, "server_error", "授权码状态不可用")
		return
	}
	if !consumed {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "授权码已被使用")
		return
	}

	user, err := p.users.ByID(ctx, grant.UserID)
	if err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "用户不存在")
		return
	}
	if user.Status == model.UserStatusDisabled {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "用户已被禁用")
		return
	}

	p.issueTokens(c, client, user, grant.Scopes, grant.AuthTime, grant.Nonce)
}

func (p *Provider) exchangeRefresh(c *gin.Context, client *model.OAuthClient, form url.Values) {
	ctx := c.Request.Context()
	raw := form.Get("refresh_token")
	if raw == "" {
		tokenError(c, http.StatusBadRequest, "invalid_request", "缺少 refresh_token")
		return
	}

	grant, err := p.refresh.Peek(ctx, raw)
	if err != nil {
		if errors.Is(err, ErrRefreshInvalid) {
			tokenError(c, http.StatusBadRequest, "invalid_grant", "刷新令牌无效或已过期")
			return
		}
		tokenError(c, http.StatusInternalServerError, "server_error", "刷新令牌状态不可用")
		return
	}
	if grant.ClientID != client.ClientID {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "刷新令牌与客户端不匹配")
		return
	}

	scopes := grant.Scopes
	if requested := splitScope(form.Get("scope")); len(requested) > 0 {
		if !scopeSubset(requested, grant.Scopes) {
			tokenError(c, http.StatusBadRequest, "invalid_scope", "请求的 scope 超出原授权范围")
			return
		}
		scopes = requested
	}

	user, err := p.users.ByID(ctx, grant.UserID)
	if err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "用户不存在")
		return
	}
	if user.Status == model.UserStatusDisabled {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "用户已被禁用")
		return
	}

	permitted, err := p.clients.Allows(ctx, client, user.ID)
	if err != nil {
		tokenError(c, http.StatusInternalServerError, "server_error", "应用权限状态不可用")
		return
	}
	if !permitted {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "用户已无权访问该应用")
		return
	}

	previous, err := p.refresh.Consume(ctx, raw)
	if err != nil {
		if errors.Is(err, ErrRefreshInvalid) {
			tokenError(c, http.StatusBadRequest, "invalid_grant", "刷新令牌无效或已使用")
			return
		}
		tokenError(c, http.StatusInternalServerError, "server_error", "刷新令牌状态不可用")
		return
	}

	p.issueTokens(c, client, user, scopes, previous.AuthTime, "")
}

// issueTokens 同时签发访问令牌、ID 令牌与刷新令牌，刷新令牌每次轮转。
// 刷新路径不回传 nonce，OIDC Core 12.2 要求重新签发的 ID 令牌不带 nonce。
func (p *Provider) issueTokens(c *gin.Context, client *model.OAuthClient, user *model.User, scopes []string, authTime int64, nonce string) {
	ctx := c.Request.Context()
	now := time.Now()
	audience := []string{client.ClientID}

	refreshToken, err := p.refresh.Issue(ctx, RefreshGrant{
		UserID:   user.ID,
		ClientID: client.ClientID,
		Scopes:   scopes,
		AuthTime: authTime,
	})
	if err != nil {
		tokenError(c, http.StatusInternalServerError, "server_error", "签发刷新令牌失败")
		return
	}

	accessToken, err := p.sign(jwt.MapClaims{
		"iss":       p.Issuer,
		"sub":       user.ID,
		"aud":       audience,
		"iat":       now.Unix(),
		"exp":       now.Add(accessTokenTTL).Unix(),
		"scope":     strings.Join(scopes, " "),
		"token_use": "access",
	})
	if err != nil {
		tokenError(c, http.StatusInternalServerError, "server_error", "签发访问令牌失败")
		return
	}

	idClaims := jwt.MapClaims{
		"iss":       p.Issuer,
		"aud":       audience,
		"iat":       now.Unix(),
		"exp":       now.Add(accessTokenTTL).Unix(),
		"auth_time": authTime,
	}
	for key, value := range p.claimsFor(ctx, user, scopes) {
		idClaims[key] = value
	}
	if nonce != "" {
		idClaims["nonce"] = nonce
	}

	idToken, err := p.sign(idClaims)
	if err != nil {
		tokenError(c, http.StatusInternalServerError, "server_error", "签发 ID 令牌失败")
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(accessTokenTTL.Seconds()),
		"id_token":      idToken,
		"refresh_token": refreshToken,
		"scope":         strings.Join(scopes, " "),
	})
}

func clientCredentials(c *gin.Context, form url.Values) (string, string, bool) {
	if id, secret, ok := c.Request.BasicAuth(); ok {
		return id, secret, true
	}
	if id := form.Get("client_id"); id != "" {
		return id, form.Get("client_secret"), true
	}
	return "", "", false
}

func secretMatches(client *model.OAuthClient, secret string) bool {
	if secret == "" || client.SecretHash == "" {
		return false
	}
	matched, err := cryptox.VerifyPassword(client.SecretHash, secret)
	return err == nil && matched
}

// dummyHash 让未知 client_id 的请求也付出一次 Argon2id 计算，
// 使令牌端点各失败分支耗时一致，避免按响应时间枚举已注册的客户端。
var dummyHash = sync.OnceValue(func() string {
	hash, err := cryptox.HashPassword(uuid.NewString())
	if err != nil {
		return ""
	}
	return hash
})

func dummyVerify(secret string) {
	if hash := dummyHash(); hash != "" {
		_, _ = cryptox.VerifyPassword(hash, secret)
	}
}

func verifyPKCE(grant *Grant, verifier string) bool {
	if grant.CodeChallenge == "" {
		return true
	}
	if verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	encoded := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(encoded), []byte(grant.CodeChallenge)) == 1
}

func tokenError(c *gin.Context, status int, code, description string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"error": code, "error_description": description})
}

func (p *Provider) countFailure(ctx context.Context, c *gin.Context) {
	if p.limiter == nil {
		return
	}
	if err := p.limiter.Hit(ctx, c.ClientIP()); err != nil {
		log.Printf("oidc: 记录令牌端点失败次数: %v", err)
	}
}

func nowUnix() int64 {
	return time.Now().Unix()
}
