package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

type Events struct {
	events *repo.EventRepo
}

func NewEvents(events *repo.EventRepo) *Events {
	return &Events{events: events}
}

type eventView struct {
	ID         uint64    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Kind       string    `json:"kind"`
	UserID     string    `json:"user_id,omitempty"`
	ActorID    string    `json:"actor_id,omitempty"`
	ClientID   string    `json:"client_id,omitempty"`
	ClientName string    `json:"client_name,omitempty"`
	Method     string    `json:"method,omitempty"`
	Result     string    `json:"result"`
	Reason     string    `json:"reason,omitempty"`
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	Scopes     []string  `json:"scopes,omitempty"`
}

func (h *Events) Mine(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	filter := repo.EventFilter{UserID: user.ID}
	applyEventPaging(c, &filter)
	h.respond(c, filter)
}

func (h *Events) Admin(c *gin.Context) {
	filter := repo.EventFilter{
		UserID:   c.Query("user_id"),
		ClientID: c.Query("client_id"),
		Kind:     c.Query("kind"),
		Result:   c.Query("result"),
	}
	applyEventPaging(c, &filter)

	from, ok := parseEventTime(c.Query("from"), false)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from 时间格式不正确"})
		return
	}
	to, ok := parseEventTime(c.Query("to"), true)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to 时间格式不正确"})
		return
	}
	filter.From = from
	filter.To = to

	h.respond(c, filter)
}

func (h *Events) respond(c *gin.Context, filter repo.EventFilter) {
	items, err := h.events.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询记录失败"})
		return
	}

	views := make([]eventView, 0, len(items))
	for _, item := range items {
		views = append(views, eventView{
			ID:         item.ID,
			OccurredAt: item.OccurredAt,
			Kind:       item.Kind,
			UserID:     item.UserID,
			ActorID:    item.ActorID,
			ClientID:   item.ClientID,
			ClientName: item.ClientName,
			Method:     item.Method,
			Result:     item.Result,
			Reason:     item.Reason,
			IP:         item.IP,
			UserAgent:  item.UserAgent,
			Scopes:     item.Scopes,
		})
	}

	next := uint64(0)
	if len(items) == filter.Limit {
		next = items[len(items)-1].ID
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"events": views, "next_cursor": next})
}

func applyEventPaging(c *gin.Context, filter *repo.EventFilter) {
	filter.Limit = 50
	if value, err := strconv.Atoi(c.Query("limit")); err == nil && value > 0 && value <= 200 {
		filter.Limit = value
	}
	if value, err := strconv.ParseUint(c.Query("cursor"), 10, 64); err == nil {
		filter.Cursor = value
	}
}

func parseEventTime(raw string, endOfDay bool) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	if value, err := time.Parse(time.RFC3339, raw); err == nil {
		return &value, true
	}
	value, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, false
	}
	if endOfDay {
		value = value.Add(24*time.Hour - time.Second)
	}
	return &value, true
}
