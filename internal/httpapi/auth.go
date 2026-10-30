package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/auth"
	"github.com/thun888/gzhu-sso-relay/internal/session"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

const (
	sessionCookie = session.CookieName
	ctxUserKey    = "relay.user"
)

type Auth struct {
	service *auth.Service
	secure  bool
}

func NewAuth(service *auth.Service, secure bool) *Auth {
	return &Auth{service: service, secure: secure}
}

type userView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Type  int16  `json:"type"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Auth) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名与密码不能为空"})
		return
	}

	result, err := h.service.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		status, message := authError(err)
		c.JSON(status, gin.H{"error": message})
		return
	}

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

func (h *Auth) Me(c *gin.Context) {
	user, ok := c.Get(ctxUserKey)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	c.JSON(http.StatusOK, toUserView(user.(*model.User)))
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
	case errors.Is(err, auth.ErrUpstreamUnavailable):
		return http.StatusBadGateway, "上游认证服务不可用"
	case errors.Is(err, auth.ErrNoEmailDomain):
		return http.StatusInternalServerError, "该学号没有匹配的邮箱域名配置"
	default:
		return http.StatusInternalServerError, "登录失败"
	}
}
