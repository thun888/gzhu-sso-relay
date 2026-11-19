package oidc

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

// Introspect 实现 RFC 7662，供资源服务器在本地验签之外确认授权是否仍然有效。
func (p *Provider) Introspect(c *gin.Context) {
	ctx := c.Request.Context()
	if err := c.Request.ParseForm(); err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_request", "表单无法解析")
		return
	}
	form := c.Request.PostForm

	clientID, secret, ok := clientCredentials(c, form)
	if !ok || clientID == "" {
		tokenError(c, http.StatusUnauthorized, "invalid_client", "缺少客户端凭据")
		return
	}
	client, err := p.clients.ByClientID(ctx, clientID)
	if err != nil || !client.Enabled {
		tokenError(c, http.StatusUnauthorized, "invalid_client", "未知的客户端")
		return
	}
	if !client.IsPublic && !secretMatches(client, secret) {
		tokenError(c, http.StatusUnauthorized, "invalid_client", "客户端密钥不正确")
		return
	}

	raw := strings.TrimSpace(form.Get("token"))
	if raw == "" {
		introspectInactive(c)
		return
	}
	claims, err := p.verify(raw)
	if err != nil {
		introspectInactive(c)
		return
	}
	if use, _ := claims["token_use"].(string); use != "access" {
		introspectInactive(c)
		return
	}
	if audienceOf(claims) != client.ClientID {
		introspectInactive(c)
		return
	}

	revoked, err := p.revoked.Revoked(ctx, raw)
	if err != nil || revoked {
		introspectInactive(c)
		return
	}

	sub, _ := claims["sub"].(string)
	user, err := p.users.ByID(ctx, sub)
	if err != nil || user.Status == model.UserStatusDisabled {
		introspectInactive(c)
		return
	}

	version := int64(1)
	if value, ok := claims["ver"].(float64); ok {
		version = int64(value)
	}
	current, active, err := p.consents.ActiveVersion(ctx, sub, client.ClientID)
	if err != nil || !active || current != version {
		introspectInactive(c)
		return
	}

	expiry, _ := claims.GetExpirationTime()
	issued, _ := claims["iat"].(float64)
	scope, _ := claims["scope"].(string)

	payload := gin.H{
		"active":     true,
		"scope":      scope,
		"client_id":  client.ClientID,
		"sub":        sub,
		"token_type": "Bearer",
		"iss":        p.Issuer,
	}
	if expiry != nil {
		payload["exp"] = expiry.Unix()
	}
	if issued > 0 {
		payload["iat"] = int64(issued)
	}
	if hasScope(splitScope(scope), "profile") {
		payload["username"] = user.ID
		payload["name"] = user.Name
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, payload)
}

func introspectInactive(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"active": false})
}
