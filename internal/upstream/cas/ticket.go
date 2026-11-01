package cas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const serviceValidatePath = "/cas/serviceValidate"

var (
	ErrNoSession     = errors.New("cas: 上游会话已失效")
	ErrTicketInvalid = errors.New("cas: 上游票据校验未通过")

	principalPattern = regexp.MustCompile(`(?s)<cas:user>\s*([^<]*?)\s*</cas:user>`)
)

// ServiceTicket 借当前 SSO 会话为 service 申请一张票据，不跟进跳转。
// 上游会话不存在时返回 ErrNoSession。
func (c *Client) ServiceTicket(ctx context.Context, service string) (string, error) {
	hc := *c.HTTP
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	endpoint := strings.TrimSuffix(c.BaseURL, "/") + loginPath + "?service=" + url.QueryEscape(service)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.agent())

	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("cas: 申请票据: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return "", ErrNoSession
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return "", fmt.Errorf("cas: 解析票据跳转: %w", err)
	}
	ticket := location.Query().Get("ticket")
	if ticket == "" {
		return "", ErrNoSession
	}
	return ticket, nil
}

// Principal 校验票据并返回上游账号，即登录时使用的学号或教工号。
func (c *Client) Principal(ctx context.Context, service, ticket string) (string, error) {
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + serviceValidatePath +
		"?service=" + url.QueryEscape(service) + "&ticket=" + url.QueryEscape(ticket)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.agent())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("cas: 校验票据: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("cas: 读取票据校验结果: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cas: 校验票据返回 %d", resp.StatusCode)
	}

	page := string(body)
	if strings.Contains(page, "authenticationFailure") {
		return "", ErrTicketInvalid
	}
	m := principalPattern.FindStringSubmatch(page)
	if len(m) < 2 || strings.TrimSpace(m[1]) == "" {
		return "", ErrTicketInvalid
	}
	return strings.TrimSpace(m[1]), nil
}
