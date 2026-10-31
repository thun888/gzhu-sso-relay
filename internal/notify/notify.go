package notify

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/thun888/gzhu-sso-relay/internal/mail"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

const sendTimeout = 30 * time.Second

type Notifier struct {
	sender  *mail.Sender
	notices *repo.NoticeRepo
	site    string
}

func New(sender *mail.Sender, notices *repo.NoticeRepo, site string) *Notifier {
	return &Notifier{sender: sender, notices: notices, site: site}
}

// FirstLogin 在用户完成对某应用的授权后调用，首次授权时异步发送邮件通知。
func (n *Notifier) FirstLogin(user *model.User, client *model.OAuthClient) {
	if !n.sender.Enabled() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		defer cancel()

		first, err := n.notices.MarkFirst(ctx, user.ID, client.ClientID)
		if err != nil {
			log.Printf("notify: 记录首次登录: %v", err)
			return
		}
		if !first {
			return
		}
		if err := n.sender.Send(user.Email, n.subject(client), n.body(user, client)); err != nil {
			log.Printf("notify: 发送首次登录通知到 %s: %v", user.Email, err)
		}
	}()
}

func (n *Notifier) subject(client *model.OAuthClient) string {
	title := n.site
	if title == "" {
		title = "统一身份认证"
	}
	return fmt.Sprintf("【%s】首次登录「%s」通知", title, client.Name)
}

func (n *Notifier) body(user *model.User, client *model.OAuthClient) string {
	title := n.site
	if title == "" {
		title = "统一身份认证"
	}
	return fmt.Sprintf("%s（%s）你好：\n\n你的账号于 %s 首次授权登录应用「%s」。\n\n如非本人操作，请立即修改密码并联系管理员。\n\n%s",
		user.Name, user.ID, time.Now().Format("2006-01-02 15:04:05"), client.Name, title)
}
