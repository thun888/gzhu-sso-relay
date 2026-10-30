package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/oidc"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type Consent struct {
	pending  *oidc.PendingStore
	consents *repo.ConsentRepo
	clients  *repo.ClientRepo
	scopes   *repo.ScopeRepo
}

func NewConsent(pending *oidc.PendingStore, consents *repo.ConsentRepo, clients *repo.ClientRepo, scopes *repo.ScopeRepo) *Consent {
	return &Consent{pending: pending, consents: consents, clients: clients, scopes: scopes}
}

type scopeView struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

func (h *Consent) Show(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	ctx := c.Request.Context()
	pending, err := h.pending.Load(ctx, c.Query("req"))
	if err != nil {
		c.JSON(http.StatusGone, gin.H{"error": "授权请求已过期，请重新发起"})
		return
	}
	if pending.UserID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "授权请求不属于当前用户"})
		return
	}

	client, err := h.clients.ByClientID(ctx, pending.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "应用信息不可用"})
		return
	}

	details, err := h.scopes.ByNames(ctx, pending.Scopes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询 scope 失败"})
		return
	}

	known := make(map[string]model.Scope, len(details))
	for _, scope := range details {
		known[scope.Name] = scope
	}

	views := make([]scopeView, 0, len(pending.Scopes))
	for _, name := range pending.Scopes {
		view := scopeView{Name: name, DisplayName: name}
		if scope, ok := known[name]; ok {
			view.DisplayName = scope.DisplayName
			view.Description = scope.Description
		}
		views = append(views, view)
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"client": gin.H{"id": client.ClientID, "name": client.Name},
		"scopes": views,
	})
}

func (h *Consent) Approve(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		Req string `json:"req" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	ctx := c.Request.Context()
	pending, err := h.pending.Load(ctx, req.Req)
	if err != nil {
		c.JSON(http.StatusGone, gin.H{"error": "授权请求已过期，请重新发起"})
		return
	}
	if pending.UserID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "授权请求不属于当前用户"})
		return
	}

	if err := h.consents.Grant(ctx, user.ID, pending.ClientID, pending.Scopes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "记录授权失败"})
		return
	}
	_ = h.pending.Delete(ctx, req.Req)

	c.JSON(http.StatusOK, gin.H{"continue": pending.AuthorizeURL})
}

// currentUser 取 RequireUser 中间件写入的登录用户。
func currentUser(c *gin.Context) (*model.User, bool) {
	value, ok := c.Get(ctxUserKey)
	if !ok {
		return nil, false
	}
	user, ok := value.(*model.User)
	return user, ok
}
