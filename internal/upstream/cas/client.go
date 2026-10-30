package cas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	DefaultBaseURL = "https://newcas.gzhu.edu.cn"
	loginPath      = "/cas/login"
	portalTitle    = "融合门户"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

var (
	ErrInvalidCredentials = errors.New("cas: 账号或密码错误")

	ltPattern        = regexp.MustCompile(`name="lt"\s+value="([^"]*)"`)
	executionPattern = regexp.MustCompile(`name="execution"\s+value="([^"]*)"`)
	titlePattern     = regexp.MustCompile(`(?is)<title>\s*(.*?)\s*</title>`)
	passwordPattern  = regexp.MustCompile(`name="password"`)
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{BaseURL: DefaultBaseURL, HTTP: hc}
}

func (c *Client) Login(ctx context.Context, username, password string) error {
	lt, execution, err := c.issueToken(ctx)
	if err != nil {
		return err
	}

	form := url.Values{
		"rsa":       {strEnc(username + password + lt)},
		"ul":        {strconv.Itoa(utf8.RuneCountInString(username))},
		"pl":        {strconv.Itoa(utf8.RuneCountInString(password))},
		"lt":        {lt},
		"execution": {execution},
		"_eventId":  {"submit"},
	}

	endpoint := strings.TrimSuffix(c.BaseURL, "/") + loginPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("cas: 提交登录请求: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("cas: 读取登录响应: %w", err)
	}

	page := string(body)
	if passwordPattern.MatchString(page) {
		return ErrInvalidCredentials
	}
	if title := titlePattern.FindStringSubmatch(page); len(title) < 2 || strings.TrimSpace(title[1]) != portalTitle {
		return fmt.Errorf("cas: 登录后未进入%s，页面标题为 %q", portalTitle, firstMatch(titlePattern, page))
	}
	return nil
}

func (c *Client) issueToken(ctx context.Context) (lt, execution string, err error) {
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + loginPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("cas: 拉取登录页: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", fmt.Errorf("cas: 读取登录页: %w", err)
	}

	lt = firstMatch(ltPattern, string(body))
	if lt == "" {
		return "", "", errors.New("cas: 登录页中未找到 lt")
	}
	execution = firstMatch(executionPattern, string(body))
	if execution == "" {
		execution = "e1s1"
	}
	return lt, execution, nil
}

func firstMatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}
