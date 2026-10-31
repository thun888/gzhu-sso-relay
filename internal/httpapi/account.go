package httpapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/binding"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type Account struct {
	users    *repo.UserRepo
	accounts *repo.AccountRepo
	groups   *repo.GroupRepo
	registry *binding.Registry
	acls     *repo.BindingACLRepo
	states   *binding.StateStore
	baseURL  string
}

func NewAccount(users *repo.UserRepo, accounts *repo.AccountRepo, groups *repo.GroupRepo, registry *binding.Registry, acls *repo.BindingACLRepo, states *binding.StateStore, baseURL string) *Account {
	return &Account{
		users:    users,
		accounts: accounts,
		groups:   groups,
		registry: registry,
		acls:     acls,
		states:   states,
		baseURL:  strings.TrimSuffix(baseURL, "/"),
	}
}

type groupBrief struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

type bindingView struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Bound       bool   `json:"bound"`
	Account     string `json:"account,omitempty"`
}

func (h *Account) Me(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	ctx := c.Request.Context()

	groups, err := h.groups.OfUser(ctx, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户组失败"})
		return
	}
	briefs := make([]groupBrief, 0, len(groups))
	for _, group := range groups {
		briefs = append(briefs, groupBrief{ID: group.ID, Name: group.Name, Code: group.Code})
	}

	stored, err := h.accounts.BindingsOf(ctx, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询绑定失败"})
		return
	}
	bound := make(map[string]repo.Binding, len(stored))
	for _, item := range stored {
		bound[item.Provider] = item
	}

	bindings := make([]bindingView, 0, len(h.registry.List()))
	for _, provider := range h.registry.List() {
		permitted, err := h.acls.Allows(ctx, provider.Name, user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询绑定源权限失败"})
			return
		}
		if !permitted {
			continue
		}
		view := bindingView{Provider: provider.Name, DisplayName: provider.DisplayName}
		if item, ok := bound[provider.Name]; ok {
			view.Bound = true
			view.Account = item.DisplayName
		}
		bindings = append(bindings, view)
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"name":         user.Name,
		"email":        user.Email,
		"type":         user.Type,
		"belong_class": h.accounts.ClassOf(ctx, user.ID, repo.UpstreamProvider),
		"groups":       briefs,
		"bindings":     bindings,
	})
}

func (h *Account) StartBinding(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	name := c.Param("provider")
	if _, ok := h.registry.Get(name); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未配置的绑定源"})
		return
	}

	permitted, err := h.acls.Allows(c.Request.Context(), name, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询绑定源权限失败"})
		return
	}
	if !permitted {
		c.JSON(http.StatusForbidden, gin.H{"error": "当前账号无权使用该绑定源"})
		return
	}

	state, err := h.states.Issue(c.Request.Context(), binding.State{
		Provider: name,
		UserID:   user.ID,
		Next:     safeNextPath(c.Query("next")),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发起绑定失败"})
		return
	}

	target, err := h.registry.AuthorizeURL(name, h.callbackURL(name), state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发起绑定失败"})
		return
	}
	c.Redirect(http.StatusFound, target)
}

// Callback 由绑定源回跳，身份来自 state，不依赖会话 cookie。
func (h *Account) Callback(c *gin.Context) {
	name := c.Param("provider")
	if _, ok := h.registry.Get(name); !ok {
		c.String(http.StatusNotFound, "未配置的绑定源")
		return
	}

	state, err := h.states.Consume(c.Request.Context(), c.Query("state"))
	if err != nil || state.Provider != name {
		c.Redirect(http.StatusFound, withQuery("/", "bind_error", "state"))
		return
	}
	if c.Query("error") != "" {
		c.Redirect(http.StatusFound, withQuery(state.Next, "bind_error", "denied"))
		return
	}

	identity, err := h.registry.Exchange(c.Request.Context(), name, c.Query("code"), h.callbackURL(name))
	if err != nil {
		c.Redirect(http.StatusFound, withQuery(state.Next, "bind_error", "exchange"))
		return
	}

	err = h.accounts.Bind(c.Request.Context(), state.UserID, name, identity.ProviderUserID, identity.DisplayName, identity.Raw)
	if err != nil {
		c.Redirect(http.StatusFound, withQuery(state.Next, "bind_error", "store"))
		return
	}

	c.Redirect(http.StatusFound, withQuery(state.Next, "bound", name))
}

func (h *Account) Unbind(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	name := c.Param("provider")
	if _, ok := h.registry.Get(name); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未配置的绑定源"})
		return
	}

	if err := h.accounts.Unbind(c.Request.Context(), user.ID, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解除绑定失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Account) callbackURL(name string) string {
	return h.baseURL + "/bindings/" + url.PathEscape(name) + "/callback"
}

// safeNextPath 只允许站内路径，避免绑定完成后被带去外部站点。
func safeNextPath(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	if strings.ContainsAny(raw, "\\\r\n") {
		return "/"
	}
	return raw
}

func withQuery(base, key, value string) string {
	target, err := url.Parse(safeNextPath(base))
	if err != nil {
		target = &url.URL{Path: "/"}
	}
	query := target.Query()
	query.Set(key, value)
	target.RawQuery = query.Encode()
	return target.String()
}
