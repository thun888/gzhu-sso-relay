package httpapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// 登录页需要加载极验脚本，其余资源一律同源。
const geetestOrigins = "https://*.geetest.com https://*.geevisit.com https://*.gsensebot.com"

// 极验 v4 的挑战接口在 geetest、geevisit、gsensebot 三个域名间依次重试，
// 只放行 static.geetest.com 会让后两个备用域名被拦。
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' " + geetestOrigins + "; " +
	"style-src 'self' 'unsafe-inline' " + geetestOrigins + "; " +
	"img-src 'self' data: blob: " + geetestOrigins + "; " +
	"font-src 'self' data: " + geetestOrigins + "; " +
	"connect-src 'self' " + geetestOrigins + "; " +
	"frame-src 'self' " + geetestOrigins + "; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"object-src 'none'"

// WithSecurityHeaders 给所有响应补上基础安全响应头，HSTS 只在 base_url 为 HTTPS 时下发。
// 挂在最外层：前端静态页不经过 gin，放在中间件里会漏掉登录页与管理页。
// X-Robots-Tag 与页面 meta、robots.txt 三处同时生效，避免只靠其中一种被绕过。
func WithSecurityHeaders(secure bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		header.Set("Content-Security-Policy", contentSecurityPolicy)
		header.Set("X-Robots-Tag", "noindex, nofollow, noarchive, nosnippet")
		if secure {
			header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// OriginGuard 拦截跨站发起的状态变更请求。
// 校内应用与本站同属一个注册域，被攻陷的子域发起请求时 cookie 仍会带上，
// 仅靠 SameSite 不足以挡住，因此对 /api 的写请求额外比对 Origin。
func OriginGuard(baseURL string) gin.HandlerFunc {
	allowed := ""
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		allowed = parsed.Scheme + "://" + parsed.Host
	}

	return func(c *gin.Context) {
		if allowed == "" || !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Next()
			return
		}
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin == "" {
			referer := c.GetHeader("Referer")
			if referer == "" {
				c.Next()
				return
			}
			parsed, err := url.Parse(referer)
			if err != nil {
				rejectOrigin(c)
				return
			}
			origin = parsed.Scheme + "://" + parsed.Host
		}
		if !strings.EqualFold(origin, allowed) {
			rejectOrigin(c)
			return
		}
		c.Next()
	}
}

func rejectOrigin(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "来源校验失败"})
}
