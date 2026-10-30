package binding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var ErrUnknownProvider = errors.New("binding: 未配置的绑定源")

// Provider 用标准 OAuth2 授权码流程描述一个可绑定账号的来源。
// 只要对方支持 authorization code，就能通过配置接入，无需改代码。
type Provider struct {
	Name         string   `yaml:"name"`
	DisplayName  string   `yaml:"display_name"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	AuthURL      string   `yaml:"auth_url"`
	TokenURL     string   `yaml:"token_url"`
	UserInfoURL  string   `yaml:"userinfo_url"`
	Scopes       []string `yaml:"scopes"`
	IDField      string   `yaml:"id_field"`
	NameField    string   `yaml:"name_field"`
}

func (p Provider) validate() error {
	var missing []string
	for field, value := range map[string]string{
		"name": p.Name, "client_id": p.ClientID, "client_secret": p.ClientSecret,
		"auth_url": p.AuthURL, "token_url": p.TokenURL, "userinfo_url": p.UserInfoURL,
	} {
		if value == "" {
			missing = append(missing, p.Name+"."+field)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return errors.New("binding: 绑定源缺少配置项 " + strings.Join(missing, ", "))
	}
	return nil
}

type Identity struct {
	ProviderUserID string          `json:"provider_user_id"`
	DisplayName    string          `json:"display_name"`
	Raw            json.RawMessage `json:"raw"`
}

type Registry struct {
	providers map[string]Provider
	order     []string
	http      *http.Client
}

func NewRegistry(providers []Provider, hc *http.Client) (*Registry, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	registry := &Registry{providers: make(map[string]Provider, len(providers)), http: hc}

	for _, p := range providers {
		if err := p.validate(); err != nil {
			return nil, err
		}
		if _, dup := registry.providers[p.Name]; dup {
			return nil, fmt.Errorf("binding: 绑定源 %s 重复", p.Name)
		}
		registry.providers[p.Name] = p
		registry.order = append(registry.order, p.Name)
	}
	return registry, nil
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) List() []Provider {
	out := make([]Provider, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.providers[name])
	}
	return out
}

func (r *Registry) AuthorizeURL(name, redirectURI, state string) (string, error) {
	p, ok := r.providers[name]
	if !ok {
		return "", ErrUnknownProvider
	}

	target, err := url.Parse(p.AuthURL)
	if err != nil {
		return "", fmt.Errorf("binding: 解析 %s 的 auth_url: %w", name, err)
	}

	query := target.Query()
	query.Set("response_type", "code")
	query.Set("client_id", p.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	if len(p.Scopes) > 0 {
		query.Set("scope", strings.Join(p.Scopes, " "))
	}
	target.RawQuery = query.Encode()

	return target.String(), nil
}

func (r *Registry) Exchange(ctx context.Context, name, code, redirectURI string) (Identity, error) {
	p, ok := r.providers[name]
	if !ok {
		return Identity{}, ErrUnknownProvider
	}

	token, err := r.exchangeToken(ctx, p, code, redirectURI)
	if err != nil {
		return Identity{}, err
	}

	body, err := r.fetchUserInfo(ctx, p, token)
	if err != nil {
		return Identity{}, err
	}

	return p.identity(body)
}

func (r *Registry) exchangeToken(ctx context.Context, p Provider, code, redirectURI string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("binding: 请求 %s 的 token: %w", p.Name, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("binding: 读取 %s 的 token 响应: %w", p.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("binding: %s 的 token 端点返回 %d", p.Name, resp.StatusCode)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("binding: 解析 %s 的 token 响应: %w", p.Name, err)
	}
	if payload.Error != "" {
		return "", fmt.Errorf("binding: %s 拒绝换取 token: %s", p.Name, payload.Error)
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("binding: %s 未返回 access_token", p.Name)
	}
	return payload.AccessToken, nil
}

func (r *Registry) fetchUserInfo(ctx context.Context, p Provider, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binding: 请求 %s 的用户信息: %w", p.Name, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("binding: 读取 %s 的用户信息: %w", p.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binding: %s 的用户信息端点返回 %d", p.Name, resp.StatusCode)
	}
	return body, nil
}

func (p Provider) identity(body []byte) (Identity, error) {
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		return Identity{}, fmt.Errorf("binding: 解析 %s 的用户信息: %w", p.Name, err)
	}

	idField := p.IDField
	if idField == "" {
		idField = "id"
	}
	providerUserID := stringify(fields[idField])
	if providerUserID == "" {
		return Identity{}, fmt.Errorf("binding: %s 的用户信息缺少字段 %s", p.Name, idField)
	}

	nameField := p.NameField
	if nameField == "" {
		nameField = idField
	}
	displayName := stringify(fields[nameField])
	if displayName == "" {
		displayName = providerUserID
	}

	return Identity{ProviderUserID: providerUserID, DisplayName: displayName, Raw: body}, nil
}

func stringify(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprint(int64(v))
		}
		return fmt.Sprint(v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}
