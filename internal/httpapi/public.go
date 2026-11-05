package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/build"
	"github.com/thun888/gzhu-sso-relay/internal/config"
	"github.com/thun888/gzhu-sso-relay/internal/passkey"
)

const commitPlaceholder = "{{commit}}"

type panelView struct {
	Enabled bool   `json:"enabled"`
	Title   string `json:"title"`
	Body    string `json:"body"`
}

type footerView struct {
	Enabled bool   `json:"enabled"`
	Text    string `json:"text"`
}

type captchaView struct {
	Enabled   bool   `json:"enabled"`
	CaptchaID string `json:"captcha_id"`
	Required  bool   `json:"required"`
}

// loginView 是登录页渲染所需的公开配置，不含任何敏感字段。
type loginView struct {
	SiteTitle string      `json:"site_title"`
	QRLogin   bool        `json:"qr_login"`
	Passkey   bool        `json:"passkey"`
	Panel     panelView   `json:"login_panel"`
	Footer    footerView  `json:"login_footer"`
	Captcha   captchaView `json:"captcha"`
}

func loginConfig(ctx context.Context, server config.Server, captcha *CaptchaPolicy, passkeys *passkey.Service, qrLogin bool, ip string) loginView {
	return loginView{
		SiteTitle: server.SiteTitle,
		QRLogin:   qrLogin,
		Passkey:   passkeys.Enabled(),
		Panel: panelView{
			Enabled: server.LoginPanel.Enabled,
			Title:   server.LoginPanel.Title,
			Body:    server.LoginPanel.Body,
		},
		Footer: footerView{
			Enabled: server.LoginFooter.Enabled,
			Text:    strings.ReplaceAll(server.LoginFooter.Text, commitPlaceholder, build.Commit),
		},
		Captcha: captchaView{
			Enabled:   captcha.Enabled(),
			CaptchaID: captcha.ID(),
			Required:  captcha.Required(ctx, ip),
		},
	}
}

// PublicConfig 输出登录页渲染所需的公开配置，页面已内联配置时作为兜底。
func PublicConfig(server config.Server, captcha *CaptchaPolicy, passkeys *passkey.Service, qrLogin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, loginConfig(c.Request.Context(), server, captcha, passkeys, qrLogin, c.ClientIP()))
	}
}

// InlineConfig 生成内联到 index.html 的配置，省掉登录页的一次回源请求。
func InlineConfig(ctx context.Context, ip string, server config.Server, captcha *CaptchaPolicy, passkeys *passkey.Service, qrLogin bool) []byte {
	payload, err := json.Marshal(loginConfig(ctx, server, captcha, passkeys, qrLogin, ip))
	if err != nil {
		return nil
	}
	return payload
}
