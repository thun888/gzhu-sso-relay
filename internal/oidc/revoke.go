package oidc

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Revoke 实现 RFC 7009。token 未知或不属于当前客户端时同样返回 200，
// 避免这个端点变成令牌有效性的探测接口。
func (p *Provider) Revoke(c *gin.Context) {
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
	if err != nil {
		p.countFailure(ctx, c)
		tokenError(c, http.StatusUnauthorized, "invalid_client", "未知的客户端")
		return
	}
	if !client.IsPublic && !secretMatches(client, secret) {
		p.countFailure(ctx, c)
		tokenError(c, http.StatusUnauthorized, "invalid_client", "客户端密钥不正确")
		return
	}

	raw := form.Get("token")
	if raw == "" {
		tokenError(c, http.StatusBadRequest, "invalid_request", "缺少 token")
		return
	}

	grant, err := p.refresh.Peek(ctx, raw)
	switch {
	case err == nil:
		if grant.ClientID == client.ClientID {
			if _, err := p.refresh.Revoke(ctx, raw); err != nil {
				tokenError(c, http.StatusInternalServerError, "server_error", "撤销失败")
				return
			}
		}
		c.Status(http.StatusOK)
		return
	case !errors.Is(err, ErrRefreshInvalid):
		tokenError(c, http.StatusInternalServerError, "server_error", "令牌状态不可用")
		return
	}

	claims, err := p.verify(raw)
	if err != nil || audienceOf(claims) != client.ClientID {
		c.Status(http.StatusOK)
		return
	}
	if use, _ := claims["token_use"].(string); use != "access" {
		c.Status(http.StatusOK)
		return
	}
	expiry, err := claims.GetExpirationTime()
	if err != nil || expiry == nil {
		c.Status(http.StatusOK)
		return
	}
	if err := p.revoked.Revoke(ctx, raw, time.Until(expiry.Time)); err != nil {
		tokenError(c, http.StatusInternalServerError, "server_error", "撤销失败")
		return
	}
	c.Status(http.StatusOK)
}
