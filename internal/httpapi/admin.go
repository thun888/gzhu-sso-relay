package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type Admin struct {
	users  *repo.UserRepo
	groups *repo.GroupRepo
}

func NewAdmin(users *repo.UserRepo, groups *repo.GroupRepo) *Admin {
	return &Admin{users: users, groups: groups}
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
	ID       uint64  `json:"id"`
	Name     string  `json:"name"`
	Code     string  `json:"code"`
	ParentID *uint64 `json:"parent_id"`
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

type membersPayload struct {
	UserIDs []string `json:"user_ids"`
}

func (h *Admin) ListUsers(c *gin.Context) {
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	users, err := h.users.List(c.Request.Context(), strings.TrimSpace(c.Query("q")), limit)
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
		out = append(out, groupView{ID: g.ID, Name: g.Name, Code: g.Code, ParentID: g.ParentID})
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

	c.JSON(http.StatusCreated, groupView{ID: group.ID, Name: group.Name, Code: group.Code, ParentID: group.ParentID})
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

	fields := map[string]any{}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Code != nil {
		fields["code"] = *req.Code
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
	members, err := h.groups.Members(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询成员失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_ids": members})
}

func (h *Admin) SetGroupMembers(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}

	var req membersPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	ctx := c.Request.Context()
	unique := make([]string, 0, len(req.UserIDs))
	seen := make(map[string]struct{}, len(req.UserIDs))
	for _, uid := range req.UserIDs {
		if uid = strings.TrimSpace(uid); uid == "" {
			continue
		}
		if _, dup := seen[uid]; dup {
			continue
		}
		seen[uid] = struct{}{}
		unique = append(unique, uid)
	}

	missing, err := h.users.MissingUsers(ctx, unique)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "校验成员失败"})
		return
	}
	if len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "以下用户不存在", "user_ids": missing})
		return
	}

	if err := h.groups.SetMembers(ctx, id, unique); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置成员失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_ids": unique})
}

func pathID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return 0, false
	}
	return id, true
}
