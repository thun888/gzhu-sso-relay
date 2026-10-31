package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/binding"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type AdminBindings struct {
	registry *binding.Registry
	acls     *repo.BindingACLRepo
	groups   *repo.GroupRepo
	users    *repo.UserRepo
}

func NewAdminBindings(registry *binding.Registry, acls *repo.BindingACLRepo, groups *repo.GroupRepo, users *repo.UserRepo) *AdminBindings {
	return &AdminBindings{registry: registry, acls: acls, groups: groups, users: users}
}

type bindingACLView struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	IsPrivate   bool              `json:"is_private"`
	GroupIDs    []uint64          `json:"group_ids"`
	Users       []repo.MemberView `json:"users"`
}

func (h *AdminBindings) List(c *gin.Context) {
	ctx := c.Request.Context()

	acls, err := h.acls.All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询绑定源访问控制失败"})
		return
	}

	out := make([]bindingACLView, 0, len(h.registry.List()))
	for _, provider := range h.registry.List() {
		view := bindingACLView{
			Name:        provider.Name,
			DisplayName: provider.DisplayName,
			GroupIDs:    []uint64{},
			Users:       []repo.MemberView{},
		}
		if acl, ok := acls[provider.Name]; ok {
			view.IsPrivate = acl.IsPrivate
			if acl.GroupIDs != nil {
				view.GroupIDs = acl.GroupIDs
			}
			users, err := h.users.MemberViews(ctx, acl.UserIDs)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "查询绑定源白名单失败"})
				return
			}
			view.Users = users
		}
		out = append(out, view)
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"providers": out})
}

type bindingACLUpdate struct {
	IsPrivate bool     `json:"is_private"`
	GroupIDs  []uint64 `json:"group_ids"`
	UserIDs   []string `json:"user_ids"`
}

func (h *AdminBindings) Update(c *gin.Context) {
	name := c.Param("name")
	provider, ok := h.registry.Get(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未配置的绑定源"})
		return
	}

	var req bindingACLUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	ctx := c.Request.Context()
	groupIDs := make([]uint64, 0, len(req.GroupIDs))
	for _, groupID := range req.GroupIDs {
		if _, err := h.groups.ByID(ctx, groupID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "存在不存在的用户组"})
			return
		}
		groupIDs = append(groupIDs, groupID)
	}

	userIDs := make([]string, 0, len(req.UserIDs))
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
		userIDs = append(userIDs, id)
	}
	missing, err := h.users.MissingUsers(ctx, userIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "校验用户失败"})
		return
	}
	if len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "以下用户不存在", "user_ids": missing})
		return
	}

	acl := model.BindingACL{
		Provider:  name,
		IsPrivate: req.IsPrivate,
		GroupIDs:  groupIDs,
		UserIDs:   userIDs,
	}
	if err := h.acls.Save(ctx, acl); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存绑定源访问控制失败"})
		return
	}

	users, err := h.users.MemberViews(ctx, userIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询绑定源白名单失败"})
		return
	}
	c.JSON(http.StatusOK, bindingACLView{
		Name:        name,
		DisplayName: provider.DisplayName,
		IsPrivate:   req.IsPrivate,
		GroupIDs:    groupIDs,
		Users:       users,
	})
}
