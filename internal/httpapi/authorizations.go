package httpapi

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/audit"
	"github.com/thun888/gzhu-sso-relay/internal/oidc"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type Authorizations struct {
	consents *repo.ConsentRepo
	clients  *repo.ClientRepo
	refresh  *oidc.RefreshStore
	audit    *audit.Recorder
}

func NewAuthorizations(consents *repo.ConsentRepo, clients *repo.ClientRepo, refresh *oidc.RefreshStore, recorder *audit.Recorder) *Authorizations {
	return &Authorizations{consents: consents, clients: clients, refresh: refresh, audit: recorder}
}

type authorizationView struct {
	ClientID    string     `json:"client_id"`
	Name        string     `json:"name"`
	HomepageURL string     `json:"homepage_url"`
	Scopes      []string   `json:"scopes"`
	GrantedAt   time.Time  `json:"granted_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

func (h *Authorizations) List(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	items, err := h.consents.ListActive(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询授权失败"})
		return
	}

	views := make([]authorizationView, 0, len(items))
	for _, item := range items {
		views = append(views, authorizationView{
			ClientID:    item.ClientID,
			Name:        item.Name,
			HomepageURL: item.HomepageURL,
			Scopes:      item.Scopes,
			GrantedAt:   item.GrantedAt,
			LastUsedAt:  item.LastUsedAt,
		})
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, views)
}

// Revoke 撤销用户对应用的授权。已签发的访问令牌靠版本号立即失效，刷新令牌一并删除。
func (h *Authorizations) Revoke(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	ctx := c.Request.Context()
	clientID := c.Param("client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少应用标识"})
		return
	}

	if err := h.consents.Revoke(ctx, user.ID, clientID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "撤销授权失败"})
		return
	}
	if _, err := h.refresh.RevokeUserClient(ctx, user.ID, clientID); err != nil {
		log.Printf("撤销授权时清理刷新令牌失败 user=%s client=%s: %v", user.ID, clientID, err)
	}

	clientName := ""
	if client, err := h.clients.ByClientID(ctx, clientID); err == nil {
		clientName = client.Name
	}
	h.audit.Record(ctx, audit.Meta{IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), Method: "consent"}, model.AuthEvent{
		Kind:       audit.KindAuthorizeRevoke,
		UserID:     user.ID,
		ClientID:   clientID,
		ClientName: clientName,
		Result:     audit.ResultSuccess,
		Reason:     "user",
	})

	c.Status(http.StatusNoContent)
}
