package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/config"
)

// PublicConfig 输出登录页渲染所需的公开配置，不含任何敏感字段。
func PublicConfig(siteTitle string, panel config.LoginPanel) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"site_title": siteTitle,
			"login_panel": gin.H{
				"enabled": panel.Enabled,
				"title":   panel.Title,
				"body":    panel.Body,
			},
		})
	}
}
