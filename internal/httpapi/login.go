package httpapi

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/auth"
)

// 该页面是前端 SPA 落地前的临时登录入口，React 上线后由 /login 路由接管。
const loginHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>登录</title>
<style>
  body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center;
         font-family: system-ui,-apple-system,"Segoe UI",sans-serif; background:#f6f7f9; color:#1d2129; }
  form { width:min(360px,90vw); background:#fff; padding:32px; border-radius:12px;
         border:1px solid #e5e6eb; display:flex; flex-direction:column; gap:14px; }
  h1 { font-size:18px; font-weight:500; margin:0 0 6px; }
  label { font-size:13px; color:#4e5969; display:flex; flex-direction:column; gap:6px; }
  input { padding:10px 12px; border:1px solid #e5e6eb; border-radius:8px; font-size:14px; }
  input:focus { outline:2px solid #c9d8ff; border-color:#94b4ff; }
  button { margin-top:6px; padding:11px; border:0; border-radius:8px; background:#1d4ed8;
           color:#fff; font-size:14px; cursor:pointer; }
  button:disabled { opacity:.6; cursor:default; }
  .err { color:#c1121f; font-size:13px; min-height:18px; }
</style>
</head>
<body>
<form method="post" action="/login">
  <h1>统一身份认证</h1>
  <label>学号<input name="username" autocomplete="username" required autofocus></label>
  <label>密码<input name="password" type="password" autocomplete="current-password" required></label>
  <input type="hidden" name="next" value="{{.Next}}">
  <div class="err">{{.Error}}</div>
  <button type="submit">登录</button>
</form>
</body>
</html>`

type LoginPage struct {
	service *auth.Service
	secure  bool
	tmpl    *template.Template
}

type loginPageData struct {
	Next  string
	Error string
}

func NewLoginPage(service *auth.Service, secure bool) (*LoginPage, error) {
	tmpl, err := template.New("login").Parse(loginHTML)
	if err != nil {
		return nil, err
	}
	return &LoginPage{service: service, secure: secure, tmpl: tmpl}, nil
}

func (h *LoginPage) Show(c *gin.Context) {
	h.render(c, http.StatusOK, loginPageData{Next: safeNext(c.Query("next"))})
}

func (h *LoginPage) Submit(c *gin.Context) {
	next := safeNext(c.PostForm("next"))
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	if username == "" || password == "" {
		h.render(c, http.StatusBadRequest, loginPageData{Next: next, Error: "用户名与密码不能为空"})
		return
	}

	result, err := h.service.Login(c.Request.Context(), username, password)
	if err != nil {
		_, message := authError(err)
		h.render(c, statusOf(err), loginPageData{Next: next, Error: message})
		return
	}

	setSessionCookie(c, result.Token, int(h.service.SessionTTL()/time.Second), h.secure)
	if next == "" {
		next = "/"
	}
	c.Redirect(http.StatusSeeOther, next)
}

func (h *LoginPage) render(c *gin.Context, status int, data loginPageData) {
	var buf bytes.Buffer
	if err := h.tmpl.Execute(&buf, data); err != nil {
		c.String(http.StatusInternalServerError, "页面渲染失败")
		return
	}
	c.Data(status, "text/html; charset=utf-8", buf.Bytes())
}

// safeNext 只接受站内绝对路径，避免登录后被重定向到外部站点。
func safeNext(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	return raw
}

func statusOf(err error) int {
	status, _ := authError(err)
	return status
}

func setSessionCookie(c *gin.Context, value string, maxAge int, secure bool) {
	c.SetCookie(sessionCookie, value, maxAge, "/", "", secure, true)
}
