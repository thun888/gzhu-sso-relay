package captcha

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrRejected    = errors.New("captcha: 验证未通过")
	ErrUnavailable = errors.New("captcha: 校验服务不可用")
)

const defaultURL = "https://gcaptcha4.geetest.com/validate"

// Answer 是前端通过验证后拿到的四个字段。
type Answer struct {
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

// Geetest 负责极验第四代的二次校验，签名密钥只在服务端使用。
type Geetest struct {
	ID     string
	Key    string
	URL    string
	Client *http.Client
}

func NewGeetest(id, key string) *Geetest {
	return &Geetest{
		ID:     id,
		Key:    key,
		URL:    defaultURL,
		Client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (g *Geetest) Verify(ctx context.Context, a Answer) error {
	if a.LotNumber == "" || a.CaptchaOutput == "" || a.PassToken == "" || a.GenTime == "" {
		return fmt.Errorf("%w: 缺少验证参数", ErrRejected)
	}

	form := url.Values{}
	form.Set("lot_number", a.LotNumber)
	form.Set("captcha_output", a.CaptchaOutput)
	form.Set("pass_token", a.PassToken)
	form.Set("gen_time", a.GenTime)
	form.Set("sign_token", sign(g.Key, a.LotNumber))

	endpoint := g.URL + "?captcha_id=" + url.QueryEscape(g.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.Client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: 上游返回 %d", ErrUnavailable, resp.StatusCode)
	}

	var out struct {
		Status string `json:"status"`
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if out.Result != "success" || out.Status != "success" {
		return fmt.Errorf("%w: %s", ErrRejected, out.Reason)
	}
	return nil
}

func sign(key, lotNumber string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(lotNumber))
	return hex.EncodeToString(mac.Sum(nil))
}
