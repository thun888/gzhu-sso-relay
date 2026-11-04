package httpapi

import (
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/auth"
	"github.com/thun888/gzhu-sso-relay/internal/captcha"
	"github.com/thun888/gzhu-sso-relay/internal/passkey"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

const (
	// passkeyLoginCookie 把登录挑战绑到发起它的浏览器，换个浏览器拿不到这次挑战。
	passkeyLoginCookie   = "relay_passkey_login"
	passkeyLoginPath     = "/api/login/passkey"
	passkeyRegCookie     = "relay_passkey_reg"
	passkeyRegPath       = "/api/passkeys"
	passkeyGenericError  = "通行密钥验证失败，请重试"
	passkeyRegisterError = "添加通行密钥失败，请重试"
)

type Passkey struct {
	service *passkey.Service
	auth    *auth.Service
	captcha *CaptchaPolicy
	secure  bool
}

func NewPasskey(service *passkey.Service, authService *auth.Service, policy *CaptchaPolicy, secure bool) *Passkey {
	return &Passkey{service: service, auth: authService, captcha: policy, secure: secure}
}

type passkeyView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Synced     bool       `json:"synced"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func toPasskeyView(item model.Passkey) passkeyView {
	return passkeyView{
		ID:         item.ID,
		Name:       item.Name,
		Synced:     item.BackupEligible,
		CreatedAt:  item.CreatedAt,
		LastUsedAt: item.LastUsedAt,
	}
}

type passkeyRequest struct {
	Captcha captcha.Answer `json:"captcha"`
	Name    string         `json:"name"`
}

// LoginOptions 发起一次无需用户名的通行密钥登录，浏览器据此弹出可选的本站密钥。
func (h *Passkey) LoginOptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	var req passkeyRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	ctx := c.Request.Context()
	ip := c.ClientIP()

	if h.captcha.Flagged(ctx, ip) {
		if err := h.captcha.Verify(ctx, req.Captcha); err != nil {
			log.Printf("通行密钥验证码校验未通过 ip=%s: %v", ip, err)
			status, message := captchaError(err)
			c.JSON(status, gin.H{"error": message, "captcha_required": true})
			return
		}
	}

	secret, err := passkey.NewSecret()
	if err != nil {
		log.Printf("生成通行密钥凭据失败 ip=%s: %v", ip, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法发起通行密钥登录"})
		return
	}

	assertion, err := h.service.BeginLogin(ctx, secret)
	if err != nil {
		log.Printf("生成通行密钥登录参数失败 ip=%s: %v", ip, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法发起通行密钥登录"})
		return
	}

	h.setCookie(c, passkeyLoginCookie, secret, passkeyLoginPath)
	c.JSON(http.StatusOK, assertion)
}

// Login 校验浏览器返回的断言；失败按 IP 累计，次数多了下一次会要求先过验证码。
func (h *Passkey) Login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	ctx := c.Request.Context()
	ip := c.ClientIP()
	secret, _ := c.Cookie(passkeyLoginCookie)

	userID, err := h.service.FinishLogin(ctx, secret, c.Request)
	if err != nil {
		log.Printf("通行密钥登录失败 ip=%s: %v", ip, err)
		h.setCookie(c, passkeyLoginCookie, "", passkeyLoginPath)
		// 挑战过期不是凭据被拒，不记入失败次数。
		if !errors.Is(err, passkey.ErrCeremony) {
			h.captcha.Flag(ctx, ip)
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": passkeyGenericError, "captcha_required": h.captcha.Flagged(ctx, ip)})
		return
	}

	result, err := h.auth.PasskeyLogin(ctx, userID)
	if err != nil {
		status, message := authError(err)
		log.Printf("通行密钥登录被拒 id=%s ip=%s: %v", userID, ip, err)
		h.setCookie(c, passkeyLoginCookie, "", passkeyLoginPath)
		c.JSON(status, gin.H{"error": message})
		return
	}

	h.setCookie(c, passkeyLoginCookie, "", passkeyLoginPath)
	h.captcha.Clear(ctx, ip)
	setSessionCookie(c, result.Token, int(h.auth.SessionTTL()/time.Second), h.secure)
	c.JSON(http.StatusOK, toUserView(result.User))
}

func (h *Passkey) List(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	items, err := h.service.List(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询通行密钥失败"})
		return
	}

	views := make([]passkeyView, 0, len(items))
	for _, item := range items {
		views = append(views, toPasskeyView(item))
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"passkeys": views})
}

// RegisterOptions 为当前用户生成添加设备的参数，设备名随挑战一起暂存，完成时不再取信前端。
func (h *Passkey) RegisterOptions(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req passkeyRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	secret, err := passkey.NewSecret()
	if err != nil {
		log.Printf("生成通行密钥凭据失败 id=%s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": passkeyRegisterError})
		return
	}

	creation, err := h.service.BeginRegistration(c.Request.Context(), user.ID, secret, req.Name)
	if err != nil {
		log.Printf("生成通行密钥注册参数失败 id=%s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": passkeyRegisterError})
		return
	}

	h.setCookie(c, passkeyRegCookie, secret, passkeyRegPath)
	c.JSON(http.StatusOK, creation)
}

func (h *Passkey) Register(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	secret, _ := c.Cookie(passkeyRegCookie)
	item, err := h.service.FinishRegistration(c.Request.Context(), user.ID, secret, c.Request)
	if err != nil {
		log.Printf("添加通行密钥失败 id=%s: %v", user.ID, err)
		h.setCookie(c, passkeyRegCookie, "", passkeyRegPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": passkeyRegisterError})
		return
	}

	h.setCookie(c, passkeyRegCookie, "", passkeyRegPath)
	c.JSON(http.StatusCreated, toPasskeyView(*item))
}

func (h *Passkey) Delete(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	removed, err := h.service.Delete(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解除通行密钥失败"})
		return
	}
	if !removed {
		c.JSON(http.StatusNotFound, gin.H{"error": "该通行密钥不存在"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Passkey) setCookie(c *gin.Context, name, value, path string) {
	maxAge := int(passkey.CeremonyTTL / time.Second)
	if value == "" {
		maxAge = -1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		Secure:   h.secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
