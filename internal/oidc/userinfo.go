package oidc

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

func (p *Provider) UserInfo(c *gin.Context) {
	raw, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(raw) == "" {
		unauthorized(c)
		return
	}

	claims, err := p.verify(strings.TrimSpace(raw))
	if err != nil {
		unauthorized(c)
		return
	}
	if use, _ := claims["token_use"].(string); use != "access" {
		unauthorized(c)
		return
	}

	revoked, err := p.revoked.Revoked(c.Request.Context(), strings.TrimSpace(raw))
	if err != nil || revoked {
		unauthorized(c)
		return
	}

	sub, _ := claims["sub"].(string)
	scope, _ := claims["scope"].(string)

	client, err := p.clients.ByClientID(c.Request.Context(), audienceOf(claims))
	if err != nil || !client.Enabled {
		unauthorized(c)
		return
	}

	user, err := p.users.ByID(c.Request.Context(), sub)
	if err != nil || user.Status == model.UserStatusDisabled {
		unauthorized(c)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, p.claimsFor(c.Request.Context(), user, splitScope(scope)))
}

// audienceOf 取令牌的单一受众，即签发时的 client_id。
func audienceOf(claims jwt.MapClaims) string {
	switch value := claims["aud"].(type) {
	case string:
		return value
	case []any:
		if len(value) > 0 {
			audience, _ := value[0].(string)
			return audience
		}
	case []string:
		if len(value) > 0 {
			return value[0]
		}
	}
	return ""
}

func unauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
}
