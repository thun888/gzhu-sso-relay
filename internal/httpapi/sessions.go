package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/audit"
	"github.com/thun888/gzhu-sso-relay/internal/session"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type Sessions struct {
	manager *session.Manager
	audit   *audit.Recorder
}

func NewSessions(manager *session.Manager, recorder *audit.Recorder) *Sessions {
	return &Sessions{manager: manager, audit: recorder}
}

type sessionView struct {
	ID        string    `json:"id"`
	Method    string    `json:"method"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	Current   bool      `json:"current"`
}

func (h *Sessions) List(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	items, err := h.manager.List(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询登录设备失败"})
		return
	}

	current, _ := currentSession(c)
	views := make([]sessionView, 0, len(items))
	for _, item := range items {
		views = append(views, sessionView{
			ID:        item.ID,
			Method:    item.Method,
			IP:        item.IP,
			UserAgent: item.UserAgent,
			CreatedAt: item.CreatedAt,
			Current:   current != nil && item.ID == current.ID,
		})
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, views)
}

func (h *Sessions) Revoke(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	ctx := c.Request.Context()
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少会话标识"})
		return
	}

	revoked, err := h.manager.RevokeOne(ctx, user.ID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "下线失败"})
		return
	}
	if !revoked {
		c.JSON(http.StatusNotFound, gin.H{"error": "该设备已下线"})
		return
	}

	h.audit.Record(ctx, audit.Meta{IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), Method: "session"}, model.AuthEvent{
		Kind:      audit.KindSessionRevoke,
		UserID:    user.ID,
		ActorID:   user.ID,
		SessionID: id,
		Result:    audit.ResultSuccess,
		Reason:    "user",
	})

	c.Status(http.StatusNoContent)
}
