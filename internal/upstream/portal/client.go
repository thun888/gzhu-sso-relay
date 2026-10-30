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
)

const (
	DefaultBaseURL = "https://newmy.gzhu.edu.cn"
	profilePath    = "/up/up/gzhuStaffInfo/getPIThree"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

var ErrProfileUnavailable = errors.New("portal: 未取到 MOBILE，无法确定身份")

// Profile 是上游返回中本系统保留的字段，其余（单位、专业、类别）不进入 claims。
type Profile struct {
	Sub   string `json:"sub"`
	Name  string `json:"name"`
	Class string `json:"belong_class"`
}

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

// Bootstrap 走一次门户首页，让 CAS 票据在本域落地成门户自己的会话 cookie。
func (c *Client) Bootstrap(ctx context.Context) error {
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

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

func (c *Client) Profile(ctx context.Context) (Profile, error) {
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + profilePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader("{}"))
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

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
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Profile{}, fmt.Errorf("portal: 解析用户信息: %w: %.120s", err, body)
	}
	if raw.Mobile == "" {
		return Profile{}, fmt.Errorf("%w，返回字段: %s", ErrProfileUnavailable, jsonKeys(body))
	}

	return Profile{
		Sub:   raw.Mobile,
		Name:  raw.UserName,
		Class: raw.BelongClass,
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
