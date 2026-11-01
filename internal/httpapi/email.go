package httpapi

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/config"
	"github.com/thun888/gzhu-sso-relay/internal/emailchange"
	"github.com/thun888/gzhu-sso-relay/internal/mail"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

const emailAliasMaxLen = 64

type EmailChange struct {
	users      *repo.UserRepo
	domains    config.EmailDomains
	sender     *mail.Sender
	store      *emailchange.Store
	baseURL    string
	accountURL string
	site       string
}

func NewEmailChange(users *repo.UserRepo, domains config.EmailDomains, sender *mail.Sender, store *emailchange.Store, baseURL, accountURL, site string) *EmailChange {
	return &EmailChange{
		users:      users,
		domains:    domains,
		sender:     sender,
		store:      store,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		accountURL: accountURL,
		site:       site,
	}
}

type emailChangeRequest struct {
	Email string `json:"email" binding:"required"`
}

// Request 校验新邮箱并发送确认邮件。只允许改动 @ 前的别名，后缀由学号对应的配置决定。
func (h *EmailChange) Request(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req emailChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写新邮箱"})
		return
	}

	ctx := c.Request.Context()
	email := strings.ToLower(strings.TrimSpace(req.Email))

	suffix, ok := h.domains.Suffix(user.ID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "该学号没有匹配的邮箱域名配置"})
		return
	}
	if msg := aliasIssue(email, suffix); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if email == user.Email {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新邮箱不能与当前邮箱相同"})
		return
	}

	taken, err := h.users.EmailTaken(ctx, email, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询邮箱占用失败"})
		return
	}
	if taken {
		c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已被其他账号使用"})
		return
	}

	if !h.sender.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮件服务未配置，暂时无法修改邮箱"})
		return
	}

	token, err := h.store.Issue(ctx, emailchange.Request{
		UserID:   user.ID,
		Email:    email,
		OldEmail: user.Email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建确认请求失败"})
		return
	}

	link := h.baseURL + "/api/email/confirm?token=" + url.QueryEscape(token)
	if err := h.sender.Send(email, h.subject(), h.confirmBody(user, email, link)); err != nil {
		log.Printf("发送邮箱变更确认邮件失败 id=%s to=%s: %v", user.ID, email, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "确认邮件发送失败，请稍后重试"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Confirm 由确认邮件里的链接进入，不依赖登录态，token 即凭证。切换前重查占用，避免等待期内被他人取用。
func (h *EmailChange) Confirm(c *gin.Context) {
	req, err := h.store.Consume(c.Request.Context(), c.Query("token"))
	if err != nil {
		h.redirect(c, "invalid")
		return
	}

	ctx := c.Request.Context()
	user, err := h.users.ByID(ctx, req.UserID)
	if err != nil {
		h.redirect(c, "invalid")
		return
	}

	suffix, ok := h.domains.Suffix(user.ID)
	if !ok || aliasIssue(req.Email, suffix) != "" {
		h.redirect(c, "invalid")
		return
	}
	if req.Email == user.Email {
		h.redirect(c, "")
		return
	}

	taken, err := h.users.EmailTaken(ctx, req.Email, user.ID)
	if err != nil {
		h.redirect(c, "failed")
		return
	}
	if taken {
		h.redirect(c, "taken")
		return
	}

	if err := h.users.UpdateEmail(ctx, user.ID, req.Email); err != nil {
		log.Printf("切换邮箱别名失败 id=%s email=%s: %v", user.ID, req.Email, err)
		h.redirect(c, "failed")
		return
	}
	log.Printf("用户 %s 将邮箱从 %s 切换为 %s", user.ID, req.OldEmail, req.Email)
	h.redirect(c, "")
}

func (h *EmailChange) redirect(c *gin.Context, errCode string) {
	target, err := url.Parse(h.accountURL)
	if err != nil || target.Path == "" || !strings.HasPrefix(target.Path, "/") {
		target = &url.URL{Path: "/"}
	}
	query := target.Query()
	if errCode == "" {
		query.Set("email_changed", "1")
	} else {
		query.Set("email_error", errCode)
	}
	target.RawQuery = query.Encode()
	c.Redirect(http.StatusFound, target.String())
}

func (h *EmailChange) subject() string {
	title := h.site
	if title == "" {
		title = "统一身份认证"
	}
	return "【" + title + "】邮箱别名变更确认"
}

func (h *EmailChange) confirmBody(user *model.User, newEmail, link string) string {
	title := h.site
	if title == "" {
		title = "统一身份认证"
	}
	return fmt.Sprintf("%s（%s）你好：\n\n"+
		"你正在申请把账号邮箱变更为 %s，确认后将替代 %s，作为后续所有服务返回的邮箱。\n\n"+
		"请在 %d 分钟内点击以下链接完成确认：\n%s\n\n"+
		"如非本人操作，请忽略本邮件。\n\n%s",
		user.Name, user.ID, newEmail, user.Email, int(h.store.TTL().Minutes()), link, title)
}

func aliasIssue(email, suffix string) string {
	alias, domain, found := strings.Cut(email, "@")
	if !found || alias == "" || domain == "" {
		return "邮箱格式不正确"
	}
	if !strings.EqualFold(domain, suffix) {
		return "只能修改 @ 前的别名，后缀需为 " + suffix
	}
	if len(alias) > emailAliasMaxLen {
		return "别名长度不能超过 64 个字符"
	}
	for _, r := range alias {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
		default:
			return "别名只能包含字母、数字与 . _ -"
		}
	}
	return ""
}
