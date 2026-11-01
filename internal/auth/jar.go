package auth

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// cookieJar 只服务一轮扫码登录：cookie 随扫码记录存进 Redis，每次轮询从零重建，
// 因此按主机名与路径简单匹配即可，不需要 RFC 6265 的完整规则。
type cookieJar struct {
	cookies map[string]*http.Cookie
}

func newCookieJar(cookies []*http.Cookie) *cookieJar {
	jar := &cookieJar{cookies: make(map[string]*http.Cookie, len(cookies))}
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		jar.cookies[jarKey(cookie)] = cookie
	}
	return jar
}

func jarKey(cookie *http.Cookie) string {
	return cookie.Name + "|" + cookie.Domain + "|" + cookie.Path
}

func (j *cookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		// 上游下发的是 host-only cookie，落库时补上主机名，回放时才不会串到另一个域名。
		if cookie.Domain == "" {
			cookie.Domain = strings.ToLower(u.Hostname())
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		if cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && cookie.Expires.Before(time.Now())) {
			delete(j.cookies, jarKey(cookie))
			continue
		}
		j.cookies[jarKey(cookie)] = cookie
	}
}

func (j *cookieJar) Cookies(u *url.URL) []*http.Cookie {
	host := strings.ToLower(u.Hostname())
	path := u.Path
	if path == "" {
		path = "/"
	}

	now := time.Now()
	out := make([]*http.Cookie, 0, len(j.cookies))
	for _, cookie := range j.cookies {
		if !cookie.Expires.IsZero() && cookie.Expires.Before(now) {
			continue
		}
		if cookie.Secure && u.Scheme != "https" {
			continue
		}
		if host != cookie.Domain && !strings.HasSuffix(host, "."+cookie.Domain) {
			continue
		}
		if !strings.HasPrefix(path, cookie.Path) {
			continue
		}
		out = append(out, cookie)
	}
	return out
}

func (j *cookieJar) snapshot() []*http.Cookie {
	out := make([]*http.Cookie, 0, len(j.cookies))
	for _, cookie := range j.cookies {
		out = append(out, cookie)
	}
	return out
}
