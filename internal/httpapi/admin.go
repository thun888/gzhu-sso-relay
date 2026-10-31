package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/oidc"
	"github.com/thun888/gzhu-sso-relay/internal/session"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type Admin struct {
	users    *repo.UserRepo
	groups   *repo.GroupRepo
	sessions *session.Manager
	refresh  *oidc.RefreshStore
}

func NewAdmin(users *repo.UserRepo, groups *repo.GroupRepo, sessions *session.Manager, refresh *oidc.RefreshStore) *Admin {
	return &Admin{users: users, groups: groups, sessions: sessions, refresh: refresh}
}

type adminUserView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Type        int16  `json:"type"`
	Status      int16  `json:"status"`
	LastLoginAt string `json:"last_login_at,omitempty"`
}

type groupView struct {
	ID         uint64  `json:"id"`
	Name       string  `json:"name"`
	Code       string  `json:"code"`
	IsUpstream bool    `json:"is_upstream"`
	ParentID   *uint64 `json:"parent_id"`
}

type userPatch struct {
	Type   *int16 `json:"type"`
	Status *int16 `json:"status"`
}

type groupCreate struct {
	Name     string  `json:"name" binding:"required"`
	Code     string  `json:"code" binding:"required"`
	ParentID *uint64 `json:"parent_id"`
}

type groupPatch struct {
	Name     *string `json:"name"`
	Code     *string `json:"code"`
	ParentID *uint64 `json:"parent_id"`
}

type memberPayload struct {
	UserID string `json:"user_id" binding:"required"`
}

func (h *Admin) ListUsers(c *gin.Context) {
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	filter := repo.UserFilter{Keyword: strings.TrimSpace(c.Query("q"))}
	if raw := c.Query("type"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			value := int16(n)
			filter.Type = &value
		}
	}
	if raw := c.Query("status"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			value := int16(n)
			filter.Status = &value
		}
	}
	if raw := c.Query("group_id"); raw != "" {
		if n, err := strconv.ParseUint(raw, 10, 64); err == nil {
			filter.GroupID = n
		}
	}

	users, err := h.users.List(c.Request.Context(), filter, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}

	out := make([]adminUserView, 0, len(users))
	for _, u := range users {
		view := adminUserView{ID: u.ID, Name: u.Name, Email: u.Email, Type: u.Type, Status: u.Status}
		if u.LastLoginAt != nil {
			view.LastLoginAt = u.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, view)
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (h *Admin) PatchUser(c *gin.Context) {
	var req userPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	fields := map[string]any{}
	if req.Type != nil {
		if *req.Type != model.UserTypeNormal && *req.Type != model.UserTypeAdmin {
			c.JSON(http.StatusBadRequest, gin.H{"error": "type 只能为 0 或 1"})
			return
		}
		fields["type"] = *req.Type
	}
	if req.Status != nil {
		if *req.Status != model.UserStatusActive && *req.Status != model.UserStatusDisabled {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status 只能为 0 或 1"})
			return
		}
		fields["status"] = *req.Status
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有需要更新的字段"})
		return
	}

	if err := h.users.UpdateFlags(c.Request.Context(), c.Param("id"), fields); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新用户失败"})
		return
	}
	// 禁用后立即失效该用户所有会话与刷新令牌，否则重新启用时旧凭据会复活。
	if req.Status != nil && *req.Status == model.UserStatusDisabled {
		if _, err := h.sessions.RevokeAll(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "账号已禁用，但清理会话失败"})
			return
		}
		if _, err := h.refresh.RevokeUser(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "账号已禁用，但清理刷新令牌失败"})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *Admin) ListGroups(c *gin.Context) {
	groups, err := h.groups.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户组失败"})
		return
	}

	out := make([]groupView, 0, len(groups))
	for _, g := range groups {
		out = append(out, groupView{ID: g.ID, Name: g.Name, Code: g.Code, IsUpstream: g.IsUpstream, ParentID: g.ParentID})
	}
	c.JSON(http.StatusOK, gin.H{"groups": out})
}

func (h *Admin) CreateGroup(c *gin.Context) {
	var req groupCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 与 code 均为必填"})
		return
	}

	ctx := c.Request.Context()
	if req.ParentID != nil {
		if _, err := h.groups.ByID(ctx, *req.ParentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "父组不存在"})
			return
		}
	}

	group := model.Group{Name: req.Name, Code: req.Code, ParentID: req.ParentID}
	if err := h.groups.Create(ctx, &group); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "创建用户组失败，code 可能已存在"})
		return
	}

	c.JSON(http.StatusCreated, groupView{ID: group.ID, Name: group.Name, Code: group.Code, IsUpstream: group.IsUpstream, ParentID: group.ParentID})
}

func (h *Admin) PatchGroup(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}

	var req groupPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	if req.Code != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标识创建后不可修改"})
		return
	}

	if req.Name != nil {
		group, err := h.groups.ByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户组不存在"})
			return
		}
		if group.IsUpstream {
			c.JSON(http.StatusBadRequest, gin.H{"error": "上游同步的用户组不能修改名称"})
			return
		}
	}

	fields := map[string]any{}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.ParentID != nil {
		if *req.ParentID == 0 {
			fields["parent_id"] = nil
		} else {
			if *req.ParentID == id {
				c.JSON(http.StatusBadRequest, gin.H{"error": "父组不能是自己"})
				return
			}
			if _, err := h.groups.ByID(c.Request.Context(), *req.ParentID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "父组不存在"})
				return
			}
			fields["parent_id"] = *req.ParentID
		}
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有需要更新的字段"})
		return
	}

	if err := h.groups.Update(c.Request.Context(), id, fields); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新用户组失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Admin) DeleteGroup(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.groups.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户组失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Admin) GroupMembers(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	members, err := h.groups.MemberViews(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询成员失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

func (h *Admin) AddGroupMember(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}

	var req memberPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	ctx := c.Request.Context()
	group, err := h.groups.ByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户组不存在"})
		return
	}
	if group.IsUpstream {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上游同步的用户组不支持手工维护成员"})
		return
	}

	userID := strings.TrimSpace(req.UserID)
	if _, err := h.users.ByID(ctx, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户不存在"})
		return
	}
	if err := h.groups.AddMember(ctx, id, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "添加成员失败"})
		return
	}

	members, err := h.groups.MemberViews(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询成员失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

func (h *Admin) RemoveGroupMember(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	group, err := h.groups.ByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户组不存在"})
		return
	}
	if group.IsUpstream {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上游同步的用户组不支持手工维护成员"})
		return
	}

	if err := h.groups.RemoveMember(ctx, id, c.Param("user_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移除成员失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

func pathID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return 0, false
	}
	return id, true
}
