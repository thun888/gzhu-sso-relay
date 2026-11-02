package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/config"
	"github.com/thun888/gzhu-sso-relay/internal/confirm"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

const emailAliasMaxLen = 64

type EmailChange struct {
	users         *repo.UserRepo
	domains       config.EmailDomains
	confirmations *Confirmation
	accountURL    string
	site          string
}

func NewEmailChange(users *repo.UserRepo, domains config.EmailDomains, confirmations *Confirmation, accountURL, site string) *EmailChange {
	return &EmailChange{
		users:         users,
		domains:       domains,
		confirmations: confirmations,
		accountURL:    accountURL,
		site:          site,
	}
}

type emailChangePayload struct {
	Email    string `json:"email"`
	OldEmail string `json:"old_email"`
}

func (h *EmailChange) Flow() ConfirmFlow {
	return ConfirmFlow{
		Kind:       confirm.EmailChange,
		Action:     "修改邮箱",
		Target:     h.accountURL,
		SuccessKey: "email_changed",
		ErrorKey:   "email_error",
		Apply:      h.apply,
	}
}

// Request 校验新邮箱并发送确认邮件。只允许改动 @ 前的别名，后缀由学号对应的配置决定。
func (h *EmailChange) Request(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req emailChangePayload
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

	h.confirmations.Send(c, confirm.EmailChange, user, email,
		emailChangePayload{Email: email, OldEmail: user.Email},
		func(link string, ttl time.Duration) (string, string) {
			return h.subject(), h.body(user, email, link, ttl)
		})
}

// apply 在确认链接被访问时执行，切换前重查占用，避免等待期内被他人取用。
func (h *EmailChange) apply(c *gin.Context, user *model.User, payload json.RawMessage) string {
	var req emailChangePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return "invalid"
	}

	ctx := c.Request.Context()
	suffix, ok := h.domains.Suffix(user.ID)
	if !ok || aliasIssue(req.Email, suffix) != "" {
		return "invalid"
	}
	if req.Email == user.Email {
		return ""
	}

	taken, err := h.users.EmailTaken(ctx, req.Email, user.ID)
	if err != nil {
		return "failed"
	}
	if taken {
		return "taken"
	}

	if err := h.users.UpdateEmail(ctx, user.ID, req.Email); err != nil {
		log.Printf("切换邮箱别名失败 id=%s email=%s: %v", user.ID, req.Email, err)
		return "failed"
	}
	log.Printf("用户 %s 将邮箱从 %s 切换为 %s", user.ID, req.OldEmail, req.Email)
	return ""
}

func (h *EmailChange) subject() string {
	return "【" + siteName(h.site) + "】邮箱别名变更确认"
}

func (h *EmailChange) body(user *model.User, newEmail, link string, ttl time.Duration) string {
	return fmt.Sprintf("%s（%s）你好：\n\n"+
		"你正在申请把账号邮箱变更为 %s，确认后将替代 %s，作为后续所有服务返回的邮箱。\n\n"+
		"请在 %d 分钟内点击以下链接完成确认：\n%s\n\n"+
		"如非本人操作，请忽略本邮件。\n\n%s",
		user.Name, user.ID, newEmail, user.Email, int(ttl.Minutes()), link, siteName(h.site))
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
