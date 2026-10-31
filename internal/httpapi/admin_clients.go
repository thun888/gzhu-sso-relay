package httpapi

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/binding"
	"github.com/thun888/gzhu-sso-relay/internal/cryptox"
	"github.com/thun888/gzhu-sso-relay/internal/logo"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type AdminClients struct {
	clients  *repo.ClientRepo
	scopes   *repo.ScopeRepo
	groups   *repo.GroupRepo
	users    *repo.UserRepo
	registry *binding.Registry
	logos    *logo.Store
}

func NewAdminClients(clients *repo.ClientRepo, scopes *repo.ScopeRepo, groups *repo.GroupRepo, users *repo.UserRepo, registry *binding.Registry, logos *logo.Store) *AdminClients {
	return &AdminClients{clients: clients, scopes: scopes, groups: groups, users: users, registry: registry, logos: logos}
}

const maxLogoSize = 1 << 20

type clientView struct {
	ClientID         string            `json:"client_id"`
	Name             string            `json:"name"`
	LogoURL          string            `json:"logo_url"`
	HomepageURL      string            `json:"homepage_url"`
	Description      string            `json:"description"`
	IsPublic         bool              `json:"is_public"`
	IsPrivate        bool              `json:"is_private"`
	Enabled          bool              `json:"enabled"`
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
		logoURL := ""
		if client.LogoType != "" {
			logoURL = "/api/clients/" + url.PathEscape(client.ClientID) + "/logo"
		}
		redirects := client.RedirectURLs
		if redirects == nil {
			redirects = []string{}
		}
		bindings := client.RequiredBindings
		if bindings == nil {
			bindings = []string{}
		}
		views = append(views, clientView{
			ClientID:         client.ClientID,
			Name:             client.Name,
			LogoURL:          logoURL,
			HomepageURL:      client.HomepageURL,
			Description:      client.Description,
			IsPublic:         client.IsPublic,
			IsPrivate:        client.IsPrivate,
			Enabled:          client.Enabled,
			RedirectURLs:     redirects,
			RequiredBindings: bindings,
			Scopes:           scopes,
			GroupIDs:         groupIDs,
			Users:            users,
		})
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"clients": views})
}

type clientCreate struct {
	ClientID         string   `json:"client_id"`
	Name             string   `json:"name" binding:"required"`
	HomepageURL      string   `json:"homepage_url"`
	Description      string   `json:"description"`
	RedirectURLs     []string `json:"redirect_urls"`
	Scopes           []string `json:"scopes"`
	RequiredBindings []string `json:"required_bindings"`
	IsPublic         bool     `json:"is_public"`
	IsPrivate        bool     `json:"is_private"`
}

func (h *AdminClients) Create(c *gin.Context) {
	var req clientCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "应用名称不能为空"})
		return
	}

	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		generated, err := cryptox.NewClientID()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 client_id 失败"})
			return
		}
		clientID = generated
	} else if !validClientID(clientID) {
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
	redirects := req.RedirectURLs
	if redirects == nil {
		redirects = []string{}
	}
	bindings := req.RequiredBindings
	if bindings == nil {
		bindings = []string{}
	}

	client := model.OAuthClient{
		ClientID:         clientID,
		SecretHash:       hash,
		Name:             req.Name,
		HomepageURL:      strings.TrimSpace(req.HomepageURL),
		Description:      strings.TrimSpace(req.Description),
		RedirectURLs:     redirects,
		RequiredBindings: bindings,
		IsPublic:         req.IsPublic,
		IsPrivate:        req.IsPrivate,
		Enabled:          true,
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
	HomepageURL      *string   `json:"homepage_url"`
	Description      *string   `json:"description"`
	RedirectURLs     *[]string `json:"redirect_urls"`
	RequiredBindings *[]string `json:"required_bindings"`
	IsPublic         *bool     `json:"is_public"`
	IsPrivate        *bool     `json:"is_private"`
	Enabled          *bool     `json:"enabled"`
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
		fields["name"] = strings.TrimSpace(*req.Name)
	}
	if req.HomepageURL != nil {
		fields["homepage_url"] = strings.TrimSpace(*req.HomepageURL)
	}
	if req.Description != nil {
		fields["description"] = strings.TrimSpace(*req.Description)
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
		bindings := *req.RequiredBindings
		if bindings == nil {
			bindings = []string{}
		}
		fields["required_bindings"] = bindings
	}
	if req.IsPublic != nil {
		fields["is_public"] = *req.IsPublic
	}
	if req.IsPrivate != nil {
		fields["is_private"] = *req.IsPrivate
	}
	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
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

func (h *AdminClients) RotateSecret(c *gin.Context) {
	ctx := c.Request.Context()
	clientID := c.Param("id")

	client, err := h.clients.ByClientID(ctx, clientID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "应用不存在"})
		return
	}
	if client.IsPublic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "公开客户端没有密钥"})
		return
	}

	plain, hash, err := cryptox.NewSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成密钥失败"})
		return
	}
	if err := h.clients.Update(ctx, clientID, map[string]any{"secret_hash": hash}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新密钥失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"client_id": clientID, "client_secret": plain})
}

func (h *AdminClients) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.clients.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除应用失败"})
		return
	}
	if err := h.logos.Remove(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除图片失败"})
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
		if !validRedirect(raw) {
			return errInvalidRedirect
		}
	}
	return nil
}

// validRedirect 接受 http/https 绝对地址，也接受桌面与移动端使用的自定义 scheme。
// 带 fragment 的地址会把参数截断在客户端，一律拒绝。
func validRedirect(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Fragment != "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "javascript", "data", "file", "about", "blob", "vbscript":
		return false
	}
	return parsed.Host != "" || parsed.Path != ""
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
	errInvalidRedirect adminError = "回调地址必须是完整地址，支持 http、https 与自定义 scheme"
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

func (h *AdminClients) Logo(c *gin.Context) {
	client, err := h.clients.ByClientID(c.Request.Context(), c.Param("id"))
	if err != nil || client.LogoType == "" {
		c.Status(http.StatusNotFound)
		return
	}

	data, err := h.logos.Load(client.ClientID)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, client.LogoType, data)
}

func (h *AdminClients) UploadLogo(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.clients.ByClientID(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "应用不存在"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxLogoSize+4096)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取图片失败"})
		return
	}
	defer file.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	contentType := http.DetectContentType(head[:n])
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "只支持 PNG、JPEG、GIF、WebP"})
		return
	}

	rest, err := io.ReadAll(io.LimitReader(file, maxLogoSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取图片失败"})
		return
	}
	data := append(head[:n], rest...)
	if len(data) > maxLogoSize || int64(len(data)) != header.Size {
		c.JSON(http.StatusBadRequest, gin.H{"error": "图片不能超过 1MB"})
		return
	}

	if err := h.logos.Save(id, data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存图片失败"})
		return
	}
	if err := h.clients.Update(c.Request.Context(), id, map[string]any{"logo_type": contentType}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存图片失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logo_url": "/api/clients/" + url.PathEscape(id) + "/logo"})
}

func (h *AdminClients) DeleteLogo(c *gin.Context) {
	id := c.Param("id")
	if err := h.clients.Update(c.Request.Context(), id, map[string]any{"logo_type": ""}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移除图片失败"})
		return
	}
	if err := h.logos.Remove(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移除图片失败"})
		return
	}
	c.Status(http.StatusNoContent)
}
