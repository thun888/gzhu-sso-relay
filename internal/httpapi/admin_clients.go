package httpapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/binding"
	"github.com/thun888/gzhu-sso-relay/internal/cryptox"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type AdminClients struct {
	clients  *repo.ClientRepo
	scopes   *repo.ScopeRepo
	groups   *repo.GroupRepo
	users    *repo.UserRepo
	registry *binding.Registry
}

func NewAdminClients(clients *repo.ClientRepo, scopes *repo.ScopeRepo, groups *repo.GroupRepo, users *repo.UserRepo, registry *binding.Registry) *AdminClients {
	return &AdminClients{clients: clients, scopes: scopes, groups: groups, users: users, registry: registry}
}

type clientView struct {
	ClientID         string            `json:"client_id"`
	Name             string            `json:"name"`
	IsPublic         bool              `json:"is_public"`
	IsPrivate        bool              `json:"is_private"`
	RedirectURLs     []string          `json:"redirect_urls"`
	RequiredBindings []string          `json:"required_bindings"`
	Scopes           []string          `json:"scopes"`
	GroupIDs         []uint64          `json:"group_ids"`
	Users            []repo.MemberView `json:"users"`
}

func (h *AdminClients) List(c *gin.Context) {
	ctx := c.Request.Context()

	clients, err := h.clients.List(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询应用失败"})
		return
	}

	views := make([]clientView, 0, len(clients))
	for _, client := range clients {
		scopes, err := h.clients.ScopesOf(ctx, client.ClientID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询应用 scope 失败"})
			return
		}
		groupIDs, err := h.clients.GroupIDsOf(ctx, client.ClientID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询应用用户组失败"})
			return
		}
		users, err := h.clients.UsersOf(ctx, client.ClientID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询应用用户白名单失败"})
			return
		}
		views = append(views, clientView{
			ClientID:         client.ClientID,
			Name:             client.Name,
			IsPublic:         client.IsPublic,
			IsPrivate:        client.IsPrivate,
			RedirectURLs:     client.RedirectURLs,
			RequiredBindings: client.RequiredBindings,
			Scopes:           scopes,
			GroupIDs:         groupIDs,
			Users:            users,
		})
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"clients": views})
}

type clientCreate struct {
	ClientID         string   `json:"client_id" binding:"required"`
	Name             string   `json:"name" binding:"required"`
	RedirectURLs     []string `json:"redirect_urls"`
	Scopes           []string `json:"scopes"`
	RequiredBindings []string `json:"required_bindings"`
	IsPublic         bool     `json:"is_public"`
	IsPrivate        bool     `json:"is_private"`
}

func (h *AdminClients) Create(c *gin.Context) {
	var req clientCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_id 与 name 均为必填"})
		return
	}

	clientID := strings.TrimSpace(req.ClientID)
	if !validClientID(clientID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_id 只能包含字母、数字、下划线和连字符"})
		return
	}
	if err := h.checkRedirects(req.RedirectURLs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.checkScopes(c, req.Scopes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.checkBindings(req.RequiredBindings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var secret, hash string
	if !req.IsPublic {
		plain, hashed, err := cryptox.NewSecret()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成客户端密钥失败"})
			return
		}
		secret, hash = plain, hashed
	}

	ctx := c.Request.Context()
	client := model.OAuthClient{
		ClientID:         clientID,
		SecretHash:       hash,
		Name:             req.Name,
		RedirectURLs:     req.RedirectURLs,
		RequiredBindings: req.RequiredBindings,
		IsPublic:         req.IsPublic,
		IsPrivate:        req.IsPrivate,
	}
	if err := h.clients.Create(ctx, &client); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "创建应用失败，client_id 可能已存在"})
		return
	}
	if err := h.clients.SetScopes(ctx, clientID, req.Scopes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入 scope 失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"client_id":     clientID,
		"client_secret": secret,
	})
}

type clientPatch struct {
	Name             *string   `json:"name"`
	RedirectURLs     *[]string `json:"redirect_urls"`
	RequiredBindings *[]string `json:"required_bindings"`
	IsPublic         *bool     `json:"is_public"`
	IsPrivate        *bool     `json:"is_private"`
}

func (h *AdminClients) Patch(c *gin.Context) {
	var req clientPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	clientID := c.Param("id")
	fields := map[string]any{}

	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.RedirectURLs != nil {
		if err := h.checkRedirects(*req.RedirectURLs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		fields["redirect_urls"] = *req.RedirectURLs
	}
	if req.RequiredBindings != nil {
		if err := h.checkBindings(*req.RequiredBindings); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		fields["required_bindings"] = *req.RequiredBindings
	}
	if req.IsPublic != nil {
		fields["is_public"] = *req.IsPublic
	}
	if req.IsPrivate != nil {
		fields["is_private"] = *req.IsPrivate
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有需要更新的字段"})
		return
	}

	if err := h.clients.Update(c.Request.Context(), clientID, fields); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新应用失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AdminClients) Delete(c *gin.Context) {
	if err := h.clients.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除应用失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AdminClients) SetScopes(c *gin.Context) {
	var req struct {
		Scopes []string `json:"scopes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}
	if err := h.checkScopes(c, req.Scopes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clientID := c.Param("id")
	if err := h.clients.SetScopes(c.Request.Context(), clientID, req.Scopes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置 scope 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"scopes": req.Scopes})
}

func (h *AdminClients) SetGroups(c *gin.Context) {
	var req struct {
		GroupIDs []uint64 `json:"group_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	ctx := c.Request.Context()
	for _, groupID := range req.GroupIDs {
		if _, err := h.groups.ByID(ctx, groupID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "存在不存在的用户组"})
			return
		}
	}

	clientID := c.Param("id")
	if err := h.clients.SetGroups(ctx, clientID, req.GroupIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置用户组失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"group_ids": req.GroupIDs})
}

func (h *AdminClients) SetUsers(c *gin.Context) {
	var req struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	unique := make([]string, 0, len(req.UserIDs))
	seen := make(map[string]struct{}, len(req.UserIDs))
	for _, id := range req.UserIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	ctx := c.Request.Context()
	missing, err := h.users.MissingUsers(ctx, unique)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "校验用户失败"})
		return
	}
	if len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "以下用户不存在", "user_ids": missing})
		return
	}

	clientID := c.Param("id")
	if err := h.clients.SetUsers(ctx, clientID, unique); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置用户白名单失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_ids": unique})
}

func (h *AdminClients) Scopes(c *gin.Context) {
	scopes, err := h.scopes.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询 scope 失败"})
		return
	}

	out := make([]scopeView, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, scopeView{Name: scope.Name, DisplayName: scope.DisplayName, Description: scope.Description})
	}
	c.JSON(http.StatusOK, gin.H{"scopes": out})
}

func (h *AdminClients) checkRedirects(urls []string) error {
	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errInvalidRedirect
		}
	}
	return nil
}

func (h *AdminClients) checkScopes(c *gin.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	found, err := h.scopes.ByNames(c.Request.Context(), names)
	if err != nil {
		return errInvalidScope
	}
	if len(found) != len(names) {
		return errInvalidScope
	}
	return nil
}

func (h *AdminClients) checkBindings(names []string) error {
	for _, name := range names {
		if _, ok := h.registry.Get(name); !ok {
			return errInvalidBinding
		}
	}
	return nil
}

func validClientID(raw string) bool {
	if raw == "" || len(raw) > 64 {
		return false
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

type adminError string

func (e adminError) Error() string { return string(e) }

const (
	errInvalidRedirect adminError = "回调地址必须是完整的 http 或 https 地址"
	errInvalidScope    adminError = "存在未定义的 scope"
	errInvalidBinding  adminError = "存在未配置的绑定源"
)

func (h *AdminClients) Bindings(c *gin.Context) {
	providers := h.registry.List()
	out := make([]gin.H, 0, len(providers))
	for _, provider := range providers {
		out = append(out, gin.H{"name": provider.Name, "display_name": provider.DisplayName})
	}
	c.JSON(http.StatusOK, gin.H{"providers": out})
}
