package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type Leader struct {
	groups *repo.GroupRepo
	users  *repo.UserRepo
}

func NewLeader(groups *repo.GroupRepo, users *repo.UserRepo) *Leader {
	return &Leader{groups: groups, users: users}
}

type ledGroupView struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

func (h *Leader) Groups(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	groups, err := h.groups.LedGroups(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询管理的用户组失败"})
		return
	}

	out := make([]ledGroupView, 0, len(groups))
	for _, group := range groups {
		out = append(out, ledGroupView{ID: group.ID, Name: group.Name, Code: group.Code})
	}
	c.JSON(http.StatusOK, gin.H{"groups": out})
}

func (h *Leader) Users(c *gin.Context) {
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}

	active := int16(model.UserStatusActive)
	users, err := h.users.List(c.Request.Context(), repo.UserFilter{
		Keyword: strings.TrimSpace(c.Query("q")),
		Status:  &active,
	}, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}

	out := make([]repo.MemberView, 0, len(users))
	for _, user := range users {
		out = append(out, repo.MemberView{ID: user.ID, Name: user.Name, Email: user.Email, Status: user.Status})
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (h *Leader) Members(c *gin.Context) {
	group, ok := h.requireGroup(c)
	if !ok {
		return
	}
	h.respondMembers(c, group.ID)
}

func (h *Leader) AddMember(c *gin.Context) {
	group, ok := h.requireGroup(c)
	if !ok {
		return
	}

	var req memberPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}

	ctx := c.Request.Context()
	userID := strings.TrimSpace(req.UserID)
	user, err := h.users.ByID(ctx, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户不存在"})
		return
	}
	if user.Status == model.UserStatusDisabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该用户已被禁用"})
		return
	}
	if err := h.groups.AddMember(ctx, group.ID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "添加成员失败"})
		return
	}
	h.respondMembers(c, group.ID)
}

func (h *Leader) RemoveMember(c *gin.Context) {
	group, ok := h.requireGroup(c)
	if !ok {
		return
	}

	user, _ := currentUser(c)
	userID := c.Param("user_id")
	if userID == user.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能移除自己"})
		return
	}

	ctx := c.Request.Context()
	leader, err := h.groups.IsLeader(ctx, group.ID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询组长身份失败"})
		return
	}
	if leader {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能移除其他组长"})
		return
	}

	if err := h.groups.RemoveMember(ctx, group.ID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移除成员失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Leader) requireGroup(c *gin.Context) (*model.Group, bool) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return nil, false
	}

	id, ok := pathID(c)
	if !ok {
		return nil, false
	}

	ctx := c.Request.Context()
	group, err := h.groups.ByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户组不存在"})
		return nil, false
	}
	if group.IsUpstream {
		c.JSON(http.StatusForbidden, gin.H{"error": "上游同步的用户组不支持手工维护成员"})
		return nil, false
	}

	leader, err := h.groups.IsLeader(ctx, id, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询组长身份失败"})
		return nil, false
	}
	if !leader {
		c.JSON(http.StatusForbidden, gin.H{"error": "需要该用户组的组长权限"})
		return nil, false
	}
	return group, true
}

func (h *Leader) respondMembers(c *gin.Context, groupID uint64) {
	ctx := c.Request.Context()
	members, err := h.groups.MemberViews(ctx, groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询成员失败"})
		return
	}
	leaders, err := h.groups.Leaders(ctx, groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询组长失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members, "leaders": leaders})
}
