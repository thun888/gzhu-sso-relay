package oidc

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 与 store.Seed 写入的默认 scope 保持一致。
var supportedScopes = []string{"openid", "profile", "email", "groups"}

var supportedClaims = []string{
	"sub", "name", "preferred_username", "email", "email_verified", "belong_class", "groups",
}

func (p *Provider) Discovery(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                p.Issuer,
		"authorization_endpoint":                p.Issuer + "/oidc/authorize",
		"token_endpoint":                        p.Issuer + "/oidc/token",
		"userinfo_endpoint":                     p.Issuer + "/oidc/userinfo",
		"jwks_uri":                              p.Issuer + "/oidc/jwks",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      supportedScopes,
		"claims_supported":                      supportedClaims,
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"code_challenge_methods_supported":      []string{"S256", "plain"},
	})
}

func (p *Provider) JWKS(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, gin.H{"keys": []any{p.keys.JWK()}})
}
