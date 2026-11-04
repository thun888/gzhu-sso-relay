package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/build"
	"github.com/thun888/gzhu-sso-relay/internal/config"
	"github.com/thun888/gzhu-sso-relay/internal/passkey"
)

const commitPlaceholder = "{{commit}}"

// PublicConfig 输出登录页渲染所需的公开配置，不含任何敏感字段。
func PublicConfig(server config.Server, captcha *CaptchaPolicy, passkeys *passkey.Service, qrLogin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"site_title": server.SiteTitle,
			"qr_login":   qrLogin,
			"passkey":    passkeys.Enabled(),
			"login_panel": gin.H{
				"enabled": server.LoginPanel.Enabled,
				"title":   server.LoginPanel.Title,
				"body":    server.LoginPanel.Body,
			},
			"login_footer": gin.H{
				"enabled": server.LoginFooter.Enabled,
				"text":    strings.ReplaceAll(server.LoginFooter.Text, commitPlaceholder, build.Commit),
			},
			"captcha": gin.H{
				"enabled":    captcha.Enabled(),
				"captcha_id": captcha.ID(),
				"required":   captcha.Required(c.Request.Context(), c.ClientIP()),
			},
		})
	}
}
