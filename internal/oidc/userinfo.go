package oidc

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

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

	sub, _ := claims["sub"].(string)
	scope, _ := claims["scope"].(string)

	user, err := p.users.ByID(c.Request.Context(), sub)
	if err != nil || user.Status == model.UserStatusDisabled {
		unauthorized(c)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, p.claimsFor(c.Request.Context(), user, splitScope(scope)))
}

func unauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
}
