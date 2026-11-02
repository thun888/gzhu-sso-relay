package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/confirm"
	"github.com/thun888/gzhu-sso-relay/internal/mail"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
	"github.com/thun888/gzhu-sso-relay/internal/store/repo"
)

// ConfirmFlow 描述一个需要邮件确认的流程：确认页展示什么，点击确认后执行什么，
// 以及结果如何回跳。Apply 返回空串表示成功，否则作为 ErrorKey 的值带给前端。
type ConfirmFlow struct {
	Kind       confirm.Kind
	Action     string
	Target     string
	SuccessKey string
	ErrorKey   string
	Danger     bool
	Note       string
	Preview    func(*model.User, json.RawMessage) (string, bool)
	Apply      func(*gin.Context, *model.User, json.RawMessage) string
}

type Confirmation struct {
	users   *repo.UserRepo
	store   *confirm.Store
	sender  *mail.Sender
	baseURL string
	flows   map[confirm.Kind]ConfirmFlow
}

func NewConfirmation(users *repo.UserRepo, store *confirm.Store, sender *mail.Sender, baseURL string) *Confirmation {
	return &Confirmation{
		users:   users,
		store:   store,
		sender:  sender,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		flows:   make(map[confirm.Kind]ConfirmFlow),
	}
}

func (h *Confirmation) Register(flows ...ConfirmFlow) {
	for _, flow := range flows {
		h.flows[flow.Kind] = flow
	}
}

// Send 签发确认令牌，用 compose 生成的邮件正文把链接发到 to，成功时响应 204。
func (h *Confirmation) Send(c *gin.Context, kind confirm.Kind, user *model.User, to string, payload any, compose func(link string, ttl time.Duration) (string, string)) {
	flow, ok := h.flows[kind]
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "确认流程未注册"})
		return
	}
	if !h.sender.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮件服务未配置，暂时无法" + flow.Action})
		return
	}

	token, err := h.store.Issue(c.Request.Context(), kind, user.ID, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建确认请求失败"})
		return
	}

	link := h.baseURL + "/confirm?" + url.Values{
		"kind":  {string(kind)},
		"token": {token},
	}.Encode()
	subject, body := compose(link, h.store.TTL())
	if err := h.sender.Send(to, subject, body); err != nil {
		log.Printf("发送%s确认邮件失败 id=%s to=%s: %v", flow.Action, user.ID, to, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "确认邮件发送失败，请稍后重试"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Preview 供确认页展示将要执行的操作，不消费令牌。
func (h *Confirmation) Preview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	flow, user, payload, ok := h.resolve(c)
	if !ok {
		return
	}

	summary, valid := flow.Preview(user, payload)
	if !valid {
		c.JSON(http.StatusGone, gin.H{"error": "确认链接无效或已过期"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"action":  flow.Action,
		"summary": summary,
		"note":    flow.Note,
		"danger":  flow.Danger,
	})
}

// Submit 消费令牌并执行流程，结果通过 next 交给确认页跳转。
func (h *Confirmation) Submit(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	flow, ok := h.flows[confirm.Kind(c.Param("kind"))]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未知的确认类型"})
		return
	}

	ctx := c.Request.Context()
	userID, payload, err := h.store.Consume(ctx, flow.Kind, c.Query("token"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"next": resultURL(flow, "invalid")})
		return
	}

	user, err := h.users.ByID(ctx, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"next": resultURL(flow, "invalid")})
		return
	}

	c.JSON(http.StatusOK, gin.H{"next": resultURL(flow, flow.Apply(c, user, payload))})
}

// resolve 读取令牌与用户，失败时直接写出响应。
func (h *Confirmation) resolve(c *gin.Context) (ConfirmFlow, *model.User, json.RawMessage, bool) {
	flow, ok := h.flows[confirm.Kind(c.Param("kind"))]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未知的确认类型"})
		return ConfirmFlow{}, nil, nil, false
	}

	ctx := c.Request.Context()
	userID, payload, err := h.store.Peek(ctx, flow.Kind, c.Query("token"))
	if err != nil {
		c.JSON(http.StatusGone, gin.H{"error": "确认链接无效或已过期"})
		return ConfirmFlow{}, nil, nil, false
	}

	user, err := h.users.ByID(ctx, userID)
	if err != nil {
		c.JSON(http.StatusGone, gin.H{"error": "确认链接无效或已过期"})
		return ConfirmFlow{}, nil, nil, false
	}
	return flow, user, payload, true
}

func (h *Confirmation) Discard(ctx context.Context, kind confirm.Kind, userID string) error {
	return h.store.Discard(ctx, kind, userID)
}

func resultURL(flow ConfirmFlow, errCode string) string {
	target, err := url.Parse(flow.Target)
	if err != nil || target.Path == "" || !strings.HasPrefix(target.Path, "/") {
		target = &url.URL{Path: "/"}
	}
	query := target.Query()
	if errCode == "" {
		query.Set(flow.SuccessKey, "1")
	} else {
		query.Set(flow.ErrorKey, errCode)
	}
	target.RawQuery = query.Encode()
	return target.String()
}

func siteName(site string) string {
	if site == "" {
		return "统一身份认证"
	}
	return site
}
