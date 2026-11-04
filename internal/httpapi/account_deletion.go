package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/confirm"
	"github.com/thun888/gzhu-sso-relay/internal/oidc"
	"github.com/thun888/gzhu-sso-relay/internal/session"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type AccountDeletion struct {
	users         *repo.UserRepo
	sessions      *session.Manager
	refresh       *oidc.RefreshStore
	confirmations *Confirmation
	loginURL      string
	site          string
	secure        bool
}

func NewAccountDeletion(users *repo.UserRepo, sessions *session.Manager, refresh *oidc.RefreshStore, confirmations *Confirmation, loginURL, site string, secure bool) *AccountDeletion {
	return &AccountDeletion{
		users:         users,
		sessions:      sessions,
		refresh:       refresh,
		confirmations: confirmations,
		loginURL:      loginURL,
		site:          site,
		secure:        secure,
	}
}

func (h *AccountDeletion) Flow() ConfirmFlow {
	return ConfirmFlow{
		Kind:       confirm.AccountDelete,
		Action:     "注销账号",
		Target:     h.loginURL,
		SuccessKey: "account_deleted",
		ErrorKey:   "account_delete_error",
		Danger:     true,
		Note:       "注销后账号、第三方绑定、用户组与应用授权记录将被永久删除，且无法恢复。",
		Preview:    h.preview,
		Apply:      h.apply,
	}
}

// Request 向账号当前邮箱发送注销确认链接，确认前不做任何删除。
func (h *AccountDeletion) Request(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	if user.Type == model.UserTypeAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "管理员账号不能自助注销，请联系其他管理员处理"})
		return
	}

	h.confirmations.Send(c, confirm.AccountDelete, user, user.Email, nil,
		func(link string, ttl time.Duration) (string, string) {
			return h.subject(), h.body(user, link, ttl)
		})
}

func (h *AccountDeletion) preview(user *model.User, _ json.RawMessage) (string, bool) {
	return fmt.Sprintf("账号 %s（%s）将被永久注销", user.ID, user.Name), true
}

// apply 在确认链接被访问时执行，先吊销凭据再删数据：
// 清理失败时账号仍在，用户可以重新发起；反过来会留下指向已删账号的会话。
func (h *AccountDeletion) apply(c *gin.Context, user *model.User, _ json.RawMessage) string {
	if user.Type == model.UserTypeAdmin {
		return "forbidden"
	}

	ctx := c.Request.Context()
	if _, err := h.sessions.RevokeAll(ctx, user.ID); err != nil {
		log.Printf("注销账号时清理会话失败 id=%s: %v", user.ID, err)
		return "failed"
	}
	if _, err := h.refresh.RevokeUser(ctx, user.ID); err != nil {
		log.Printf("注销账号时清理刷新令牌失败 id=%s: %v", user.ID, err)
		return "failed"
	}
	if err := h.confirmations.Discard(ctx, confirm.EmailChange, user.ID); err != nil {
		log.Printf("注销账号时丢弃邮箱变更请求失败 id=%s: %v", user.ID, err)
	}
	if err := h.users.Purge(ctx, user.ID); err != nil {
		log.Printf("注销账号失败 id=%s: %v", user.ID, err)
		return "failed"
	}

	setSessionCookie(c, "", -1, h.secure)
	log.Printf("用户 %s（%s）已注销账号", user.ID, user.Email)
	return ""
}

func (h *AccountDeletion) subject() string {
	return "【" + siteName(h.site) + "】账号注销确认"
}

func (h *AccountDeletion) body(user *model.User, link string, ttl time.Duration) string {
	return fmt.Sprintf("%s（%s）你好：\n\n"+
		"我们收到了注销%s账号的请求。确认后将永久删除本站保存的账号记录，包括密码、通行密钥、第三方账号绑定、用户组归属、应用授权记录与登录通知记录，所有已登录的会话和已签发的令牌同时失效，删除后无法恢复。\n\n"+
		"请在 %d 分钟内打开以下链接，在确认页点击确认完成注销：\n%s\n\n"+
		"如非本人操作，请忽略本邮件，账号不会有任何变化。\n\n%s",
		user.Name, user.ID, siteName(h.site), int(ttl.Minutes()), link, siteName(h.site))
}
