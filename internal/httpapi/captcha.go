package httpapi

import (
	"context"
	"log"

	"github.com/thun888/gzhu-sso-relay/internal/auth"
	"github.com/thun888/gzhu-sso-relay/internal/captcha"
)

// CaptchaPolicy 把极验二次校验与按 IP 的失败标记串在一起，site 为空表示未启用。
type CaptchaPolicy struct {
	site  *captcha.Geetest
	guard *auth.CaptchaGuard
}

func NewCaptchaPolicy(site *captcha.Geetest, guard *auth.CaptchaGuard) *CaptchaPolicy {
	return &CaptchaPolicy{site: site, guard: guard}
}

func (p *CaptchaPolicy) Enabled() bool {
	return p != nil && p.site != nil
}

func (p *CaptchaPolicy) ID() string {
	if !p.Enabled() {
		return ""
	}
	return p.site.ID
}

// Required 表示该 IP 是否要先通过验证码才能登录。
func (p *CaptchaPolicy) Required(ctx context.Context, ip string) bool {
	if !p.Enabled() {
		return false
	}
	required, err := p.guard.Required(ctx, ip)
	if err != nil {
		log.Printf("验证码标记读取失败 ip=%s: %v", ip, err)
		return false
	}
	return required
}

func (p *CaptchaPolicy) Verify(ctx context.Context, answer captcha.Answer) error {
	return p.site.Verify(ctx, answer)
}

func (p *CaptchaPolicy) Flag(ctx context.Context, ip string) {
	if !p.Enabled() {
		return
	}
	if err := p.guard.Flag(ctx, ip); err != nil {
		log.Printf("验证码标记写入失败 ip=%s: %v", ip, err)
	}
}

func (p *CaptchaPolicy) Clear(ctx context.Context, ip string) {
	if !p.Enabled() {
		return
	}
	if err := p.guard.Clear(ctx, ip); err != nil {
		log.Printf("验证码标记清除失败 ip=%s: %v", ip, err)
	}
}
