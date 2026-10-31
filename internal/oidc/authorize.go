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
	if err != nil {
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
	if !scopeSubset(scopes, allowed) {
		redirectError(c, redirectURI, state, "invalid_scope", "应用未被允许申请该 scope")
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
		if method == "" {
			method = "plain"
		}
		if method != "S256" && method != "plain" {
			redirectError(c, redirectURI, state, "invalid_request", "不支持的 code_challenge_method")
			return
		}
	} else if client.IsPublic {
		redirectError(c, redirectURI, state, "invalid_request", "公开客户端必须携带 PKCE")
		return
	}

	code, err := p.codes.Issue(ctx, Grant{
		UserID:          user.ID,
		ClientID:        client.ClientID,
		RedirectURI:     redirectURI,
		Scopes:          scopes,
		Nonce:           query.Get("nonce"),
		CodeChallenge:   challenge,
		ChallengeMethod: method,
		AuthTime:        nowUnix(),
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

func redirectAllowed(client *model.OAuthClient, redirectURI string) bool {
	if redirectURI == "" {
		return false
	}
	for _, allowed := range client.RedirectURLs {
		if strings.EqualFold(allowed, redirectURI) {
			return true
		}
	}
	return false
}

func redirectError(c *gin.Context, redirectURI, state, code, description string) {
	target, err := url.Parse(redirectURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": code, "error_description": description})
		return
	}
	params := target.Query()
	params.Set("error", code)
	params.Set("error_description", description)
	if state != "" {
		params.Set("state", state)
	}
	target.RawQuery = params.Encode()
	c.Redirect(http.StatusFound, target.String())
}
