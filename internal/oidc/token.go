package oidc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

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
		p.countFailure(ctx, c)
		tokenError(c, http.StatusUnauthorized, "invalid_client", "未知的客户端")
		return
	}
	if !client.IsPublic && !secretMatches(client, secret) {
		p.countFailure(ctx, c)
		tokenError(c, http.StatusUnauthorized, "invalid_client", "客户端密钥不正确")
		return
	}

	if form.Get("grant_type") != "authorization_code" {
		tokenError(c, http.StatusBadRequest, "unsupported_grant_type", "仅支持 authorization_code")
		return
	}

	grant, err := p.codes.Peek(ctx, form.Get("code"))
	if err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "授权码无效或已过期")
		return
	}
	if grant.ClientID != client.ClientID {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "授权码与客户端不匹配")
		return
	}
	if got := form.Get("redirect_uri"); got != "" && got != grant.RedirectURI {
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

	now := time.Now()
	audience := []string{client.ClientID}

	accessToken, err := p.sign(jwt.MapClaims{
		"iss":       p.Issuer,
		"sub":       user.ID,
		"aud":       audience,
		"iat":       now.Unix(),
		"exp":       now.Add(accessTokenTTL).Unix(),
		"scope":     strings.Join(grant.Scopes, " "),
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
		"auth_time": grant.AuthTime,
	}
	for key, value := range p.claimsFor(ctx, user, grant.Scopes) {
		idClaims[key] = value
	}
	if grant.Nonce != "" {
		idClaims["nonce"] = grant.Nonce
	}

	idToken, err := p.sign(idClaims)
	if err != nil {
		tokenError(c, http.StatusInternalServerError, "server_error", "签发 ID 令牌失败")
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int(accessTokenTTL.Seconds()),
		"id_token":     idToken,
		"scope":        strings.Join(grant.Scopes, " "),
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

func verifyPKCE(grant *Grant, verifier string) bool {
	if grant.CodeChallenge == "" {
		return true
	}
	if verifier == "" {
		return false
	}
	if grant.ChallengeMethod == "S256" {
		sum := sha256.Sum256([]byte(verifier))
		encoded := base64.RawURLEncoding.EncodeToString(sum[:])
		return subtle.ConstantTimeCompare([]byte(encoded), []byte(grant.CodeChallenge)) == 1
	}
	return subtle.ConstantTimeCompare([]byte(verifier), []byte(grant.CodeChallenge)) == 1
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
