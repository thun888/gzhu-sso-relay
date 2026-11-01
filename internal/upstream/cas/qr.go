package cas

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	qrLoginPath = "/cas/qyQrLogin"
	qrCheckPath = "/cas/checkQRCodeScan"
)

// qrRedirectPattern 上游只在扫码确认后才返回 JSON，且结构未公开，这里只取需要的字段名。
var qrRedirectPattern = regexp.MustCompile(`"redirect_url"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// QRLoginURL 返回二维码承载的地址。扫码方打开后由上游把微信身份绑定到 uuid。
// service 为空时与上游登录页一致，登录后停在统一认证首页。
func QRLoginURL(baseURL, uuid, service string) string {
	endpoint := strings.TrimSuffix(baseURL, "/") + qrLoginPath + "?uuid=" + url.QueryEscape(uuid)
	if service != "" {
		endpoint += "&service=" + url.QueryEscape(service)
	}
	return endpoint
}

// QRScan 查询一次扫码状态，返回空字符串表示仍未确认。
func (c *Client) QRScan(ctx context.Context, uuid string) (string, error) {
	random := strconv.FormatFloat(rand.Float64(), 'f', 16, 64)
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + qrCheckPath +
		"?random=" + random + "&uuid=" + url.QueryEscape(uuid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.agent())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("cas: 查询扫码状态: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("cas: 读取扫码状态: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cas: 查询扫码状态返回 %d", resp.StatusCode)
	}

	m := qrRedirectPattern.FindStringSubmatch(string(body))
	if len(m) < 2 {
		return "", nil
	}
	redirect, err := strconv.Unquote(`"` + m[1] + `"`)
	if err != nil {
		return "", nil
	}
	if strings.HasPrefix(redirect, "/") {
		redirect = strings.TrimSuffix(c.BaseURL, "/") + redirect
	}
	return redirect, nil
}
