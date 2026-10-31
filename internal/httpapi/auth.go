package httpapi

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/auth"
	"github.com/thun888/gzhu-sso-relay/internal/captcha"
	"github.com/thun888/gzhu-sso-relay/internal/session"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

const (
	sessionCookie = session.CookieName
	ctxUserKey    = "relay.user"
)

type Auth struct {
	service *auth.Service
	captcha *CaptchaPolicy
	secure  bool
}

func NewAuth(service *auth.Service, policy *CaptchaPolicy, secure bool) *Auth {
	return &Auth{service: service, captcha: policy, secure: secure}
}

type userView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Type  int16  `json:"type"`
}

type loginRequest struct {
	Username string         `json:"username" binding:"required"`
	Password string         `json:"password" binding:"required"`
	Captcha  captcha.Answer `json:"captcha"`
}

func (h *Auth) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名与密码不能为空"})
		return
	}

	ctx := c.Request.Context()
	ip := c.ClientIP()
	guard := h.captcha.Required(ctx, ip)

	if guard {
		if err := h.captcha.Verify(ctx, req.Captcha); err != nil {
			log.Printf("验证码校验未通过 ip=%s: %v", ip, err)
			status, message := captchaError(err)
			c.JSON(status, gin.H{"error": message, "captcha_required": true})
			return
		}
	}

	result, err := h.service.Login(ctx, req.Username, req.Password)
	if err != nil {
		log.Printf("登录失败 id=%s ip=%s: %v", req.Username, ip, err)
		if errors.Is(err, auth.ErrInvalidCredentials) {
			h.captcha.Flag(ctx, ip)
			guard = h.captcha.Enabled()
		}
		status, message := authError(err)
		c.JSON(status, gin.H{"error": message, "captcha_required": guard})
		return
	}

	h.captcha.Clear(ctx, ip)
	setSessionCookie(c, result.Token, int(h.service.SessionTTL()/time.Second), h.secure)
	c.JSON(http.StatusOK, toUserView(result.User))
}

func (h *Auth) Logout(c *gin.Context) {
	token, _ := c.Cookie(sessionCookie)
	if token == "" {
		token = bearerToken(c)
	}
	if token != "" {
		if err := h.service.Logout(c.Request.Context(), token); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "退出失败"})
			return
		}
	}
	setSessionCookie(c, "", -1, h.secure)
	c.Status(http.StatusNoContent)
}

func (h *Auth) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := h.resolve(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		c.Set(ctxUserKey, user)
		c.Next()
	}
}

func (h *Auth) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := h.resolve(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if user.Type != model.UserTypeAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			return
		}
		c.Set(ctxUserKey, user)
		c.Next()
	}
}

func (h *Auth) resolve(c *gin.Context) (*model.User, error) {
	token, _ := c.Cookie(sessionCookie)
	if token == "" {
		token = bearerToken(c)
	}
	return h.service.Resolve(c.Request.Context(), token)
}

func toUserView(u *model.User) userView {
	return userView{ID: u.ID, Name: u.Name, Email: u.Email, Type: u.Type}
}

func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if after, ok := strings.CutPrefix(header, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func authError(err error) (int, string) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		return http.StatusUnauthorized, "账号或密码错误"
	case errors.Is(err, auth.ErrTooManyAttempts):
		return http.StatusTooManyRequests, "尝试次数过多，请稍后再试"
	case errors.Is(err, auth.ErrUpstreamLocked):
		return http.StatusTooManyRequests, "上游账号已被锁定，请稍后再试"
	case errors.Is(err, auth.ErrUpstreamChallenge):
		return http.StatusBadGateway, "上游要求验证码，无法自动登录，请联系管理员"
	case errors.Is(err, auth.ErrUpstreamUnavailable):
		return http.StatusBadGateway, "上游认证服务不可用"
	case errors.Is(err, auth.ErrNoEmailDomain):
		return http.StatusInternalServerError, "该学号没有匹配的邮箱域名配置"
	default:
		return http.StatusInternalServerError, "登录失败"
	}
}

func captchaError(err error) (int, string) {
	if errors.Is(err, captcha.ErrUnavailable) {
		return http.StatusBadGateway, "验证服务不可用，请稍后再试"
	}
	return http.StatusBadRequest, "请先完成验证"
}

func setSessionCookie(c *gin.Context, value string, maxAge int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
