package oidc

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

func (p *Provider) Authorize(c *gin.Context) {
	ctx := c.Request.Context()
	query := c.Request.URL.Query()

	clientID := query.Get("client_id")
	state := query.Get("state")
	redirectURI := query.Get("redirect_uri")

	client, err := p.clients.ByClientID(ctx, clientID)
	if err != nil || !client.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unauthorized_client", "error_description": "未知的 client_id"})
		return
	}
	if !redirectAllowed(client, redirectURI) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": "redirect_uri 不在允许列表"})
		return
	}

	if query.Get("response_type") != "code" {
		redirectError(c, redirectURI, state, "unsupported_response_type", "仅支持 authorization code")
		return
	}

	scopes := splitScope(query.Get("scope"))
	if !hasScope(scopes, "openid") {
		redirectError(c, redirectURI, state, "invalid_scope", "缺少 openid")
		return
	}

	allowed, err := p.clients.AllowedScopes(ctx, client.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	if missing := missingScopes(scopes, allowed); len(missing) > 0 {
		redirectError(c, redirectURI, state, "invalid_scope", "应用未被允许申请该 scope："+strings.Join(missing, " "))
		return
	}

	user, err := p.currentUser(c)
	if err != nil {
		next := url.QueryEscape(c.Request.URL.RequestURI())
		c.Redirect(http.StatusFound, p.LoginURL+"?next="+next)
		return
	}

	permitted, err := p.clients.Allows(ctx, client, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	if !permitted {
		redirectError(c, redirectURI, state, "access_denied", "当前账号无权访问该应用")
		return
	}

	for _, required := range client.RequiredBindings {
		permitted, err := p.bindings.Allows(ctx, required, user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if !permitted {
			redirectError(c, redirectURI, state, "access_denied", "该应用要求绑定 "+required+"，但当前账号无权使用该绑定源")
			return
		}
		bound, err := p.accounts.HasBinding(ctx, user.ID, required)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if !bound {
			next := url.QueryEscape(c.Request.URL.RequestURI())
			redirect := p.AccountURL + "?bind=" + url.QueryEscape(required) + "&next=" + next
			c.Redirect(http.StatusFound, redirect)
			return
		}
	}

	granted, err := p.consents.Scopes(ctx, user.ID, client.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	if !scopeSubset(scopes, granted) {
		id, err := p.pending.Save(ctx, Pending{
			UserID:       user.ID,
			ClientID:     client.ClientID,
			Scopes:       scopes,
			AuthorizeURL: c.Request.URL.RequestURI(),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		c.Redirect(http.StatusFound, p.ConsentURL+"?req="+url.QueryEscape(id))
		return
	}

	challenge := query.Get("code_challenge")
	method := query.Get("code_challenge_method")
	if challenge != "" {
		if method != "S256" {
			redirectError(c, redirectURI, state, "invalid_request", "code_challenge_method 必须为 S256")
			return
		}
	} else if client.IsPublic {
		redirectError(c, redirectURI, state, "invalid_request", "公开客户端必须携带 PKCE")
		return
	}

	code, err := p.codes.Issue(ctx, Grant{
		UserID:        user.ID,
		ClientID:      client.ClientID,
		RedirectURI:   redirectURI,
		Scopes:        scopes,
		Nonce:         query.Get("nonce"),
		CodeChallenge: challenge,
		AuthTime:      nowUnix(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	if p.notifier != nil {
		p.notifier.FirstLogin(user, client)
	}

	target, err := url.Parse(redirectURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": "redirect_uri 不是合法 URL"})
		return
	}
	params := target.Query()
	params.Set("code", code)
	if state != "" {
		params.Set("state", state)
	}
	target.RawQuery = params.Encode()

	c.Redirect(http.StatusFound, target.String())
}

// redirectAllowed 按组件比对回调地址：scheme 与 host 不区分大小写，路径与查询串必须完全一致。
// loopback 地址忽略端口，桌面客户端与原生应用每次监听的端口都不固定。
func redirectAllowed(client *model.OAuthClient, raw string) bool {
	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" {
		return false
	}
	for _, allowed := range client.RedirectURLs {
		if redirectMatches(allowed, target) {
			return true
		}
	}
	return false
}

func redirectMatches(allowed string, target *url.URL) bool {
	ref, err := url.Parse(allowed)
	if err != nil || ref.Scheme == "" {
		return false
	}
	if !strings.EqualFold(ref.Scheme, target.Scheme) || !strings.EqualFold(ref.Hostname(), target.Hostname()) {
		return false
	}
	if ref.EscapedPath() != target.EscapedPath() || ref.RawQuery != target.RawQuery {
		return false
	}
	if isLoopbackHost(ref.Hostname()) && isLoopbackHost(target.Hostname()) {
		return true
	}
	return ref.Port() == target.Port()
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

// ErrorRedirect 把错误参数附加到回调地址，state 原样回传。
func ErrorRedirect(redirectURI, state, code, description string) (string, error) {
	target, err := url.Parse(redirectURI)
	if err != nil {
		return "", err
	}
	params := target.Query()
	params.Set("error", code)
	params.Set("error_description", description)
	if state != "" {
		params.Set("state", state)
	}
	target.RawQuery = params.Encode()
	return target.String(), nil
}

func redirectError(c *gin.Context, redirectURI, state, code, description string) {
	target, err := ErrorRedirect(redirectURI, state, code, description)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": code, "error_description": description})
		return
	}
	c.Redirect(http.StatusFound, target)
}
