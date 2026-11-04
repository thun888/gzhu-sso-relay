package httpapi

import (
	"context"
	"log"

	"github.com/thun888/gzhu-sso-relay/internal/auth"
	"github.com/thun888/gzhu-sso-relay/internal/captcha"
)

// CaptchaPolicy 把极验二次校验与按 IP 的失败标记串在一起，site 为空表示未启用。
type CaptchaPolicy struct {
	site   *captcha.Geetest
	guard  *auth.CaptchaGuard
	always bool
}

func NewCaptchaPolicy(site *captcha.Geetest, guard *auth.CaptchaGuard, always bool) *CaptchaPolicy {
	return &CaptchaPolicy{site: site, guard: guard, always: always}
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
	if p.always {
		return true
	}
	return p.flagged(ctx, ip)
}

// Flagged 忽略 always，只看失败记录：通行密钥不是密码，每次都要验证码只会挡住正常用户，
// 同一处失败变多时再让验证码介入。
func (p *CaptchaPolicy) Flagged(ctx context.Context, ip string) bool {
	if !p.Enabled() {
		return false
	}
	return p.flagged(ctx, ip)
}

func (p *CaptchaPolicy) flagged(ctx context.Context, ip string) bool {
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

// Flag 记录一次失败标记，always 开启时也照常记录，供通行密钥判断是否需要验证码。
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
