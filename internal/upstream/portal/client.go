package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/thun888/gzhu-sso-relay/internal/upstream"
)

const (
	DefaultBaseURL = "https://newmy.gzhu.edu.cn"
	profilePath    = "/up/up/gzhuStaffInfo/getPIThree"
)

var ErrProfileUnavailable = errors.New("portal: 未取到 MOBILE，无法确定身份")

// Profile 是上游返回中本系统保留的字段。UserType、Unit、Major 只用于生成用户组，不进 claims。
type Profile struct {
	Sub      string `json:"sub"`
	Name     string `json:"name"`
	Class    string `json:"belong_class"`
	UserType string `json:"user_type"`
	Unit     string `json:"unit_name"`
	Major    string `json:"major"`
}

type Client struct {
	BaseURL   string
	UserAgent string
	HTTP      *http.Client
}

func NewClient(hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{BaseURL: DefaultBaseURL, UserAgent: upstream.DefaultUserAgent, HTTP: hc}
}

func (c *Client) agent() string {
	return upstream.RequestUserAgent(c.UserAgent)
}

// Bootstrap 走一次门户首页，让 CAS 票据在本域落地成门户自己的会话 cookie。
func (c *Client) Bootstrap(ctx context.Context) error {
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.agent())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("portal: 打开门户首页: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("portal: 读取门户首页: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("portal: 门户首页返回 %d", resp.StatusCode)
	}
	if strings.Contains(string(body), `name="password"`) {
		return errors.New("portal: 门户会话未建立，回到登录页")
	}
	return nil
}

// ServiceURL 沿门户首页的跳转链取出统一认证使用的 service 标识。
// 需要向统一认证自证身份时用它申请票据，应用注册的 service 只能从跳转里拿到。
func (c *Client) ServiceURL(ctx context.Context) (string, error) {
	hc := *c.HTTP
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/"
	for i := 0; i < 4; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", c.agent())

		resp, err := hc.Do(req)
		if err != nil {
			return "", fmt.Errorf("portal: 跟踪门户跳转: %w", err)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()

		if resp.StatusCode < 300 || resp.StatusCode > 399 {
			return "", errors.New("portal: 门户首页未跳转到统一认证")
		}
		next, err := resp.Request.URL.Parse(resp.Header.Get("Location"))
		if err != nil {
			return "", fmt.Errorf("portal: 解析门户跳转: %w", err)
		}
		if service := next.Query().Get("service"); service != "" {
			return service, nil
		}
		endpoint = next.String()
	}
	return "", errors.New("portal: 门户跳转链中没有 service 参数")
}

func (c *Client) Profile(ctx context.Context) (Profile, error) {
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + profilePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader("{}"))
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.agent())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Profile{}, fmt.Errorf("portal: 请求用户信息: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Profile{}, fmt.Errorf("portal: 用户信息返回 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Profile{}, fmt.Errorf("portal: 读取用户信息: %w", err)
	}

	var raw struct {
		Mobile      string `json:"MOBILE"`
		UserName    string `json:"USER_NAME"`
		BelongClass string `json:"BELONG_CLASS"`
		UserType    string `json:"USER_TYPE"`
		UnitName    string `json:"UNIT_NAME"`
		BelongMajor string `json:"BELONG_MAJOR"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Profile{}, fmt.Errorf("portal: 解析用户信息: %w: %.120s", err, body)
	}
	if raw.Mobile == "" {
		return Profile{}, fmt.Errorf("%w，返回字段: %s", ErrProfileUnavailable, jsonKeys(body))
	}

	return Profile{
		Sub:      raw.Mobile,
		Name:     raw.UserName,
		Class:    raw.BelongClass,
		UserType: raw.UserType,
		Unit:     raw.UnitName,
		Major:    raw.BelongMajor,
	}, nil
}

// jsonKeys 列出顶层字段名，用于定位上游返回结构变化，不输出字段值。
func jsonKeys(body []byte) string {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return "无（响应不是 JSON 对象）"
	}
	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "无（空对象）"
	}
	return strings.Join(names, ", ")
}
