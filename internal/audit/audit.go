package audit

import (
	"context"
	"log"
	"time"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

const (
	KindLoginSuccess    = "login.success"
	KindLoginFailure    = "login.failure"
	KindLogout          = "logout"
	KindSessionRevoke   = "session.revoke"
	KindConsentGrant    = "consent.grant"
	KindAuthorizeAllow  = "authorize.allow"
	KindAuthorizeDeny   = "authorize.deny"
	KindAuthorizeRevoke = "authorize.revoke"
	KindAppLogin        = "app.login"

	ResultSuccess = "success"
	ResultFailure = "failure"
	ResultDenied  = "denied"
)

type Meta struct {
	IP        string
	UserAgent string
	Method    string
}

type Recorder struct {
	events *repo.EventRepo
}

func New(events *repo.EventRepo) *Recorder {
	return &Recorder{events: events}
}

// Record 尽最大努力落库。审计失败不能反过来影响登录或授权，因此只记日志。
func (r *Recorder) Record(ctx context.Context, meta Meta, event model.AuthEvent) {
	if r == nil || r.events == nil {
		return
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	if event.IP == "" {
		event.IP = meta.IP
	}
	if event.UserAgent == "" {
		event.UserAgent = truncate(meta.UserAgent, 256)
	}
	if event.Method == "" {
		event.Method = meta.Method
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()

	if err := r.events.Record(ctx, &event); err != nil {
		log.Printf("audit: 写入事件 %s user=%s: %v", event.Kind, event.UserID, err)
	}
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
