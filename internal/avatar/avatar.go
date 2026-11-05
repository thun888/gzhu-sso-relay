package avatar

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

const defaultBase = "https://www.gravatar.com/avatar/"

// Resolver 按邮箱生成 gravatar 地址。base 是镜像源前缀，留空时用官方地址。
type Resolver struct {
	base string
}

func New(base string) *Resolver {
	base = strings.TrimSpace(base)
	if base == "" {
		base = defaultBase
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return &Resolver{base: base}
}

// URL 返回邮箱对应的头像地址，邮箱为空时返回空串。
func (r *Resolver) URL(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ""
	}
	sum := md5.Sum([]byte(email))
	return r.base + hex.EncodeToString(sum[:])
}
