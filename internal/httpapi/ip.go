package httpapi

import (
	"net"
	"net/http"
	"strings"
)

// ClientIPResolver 还原来访地址，判定顺序与 gin 的 ClientIP 一致：
// 只有对端在可信代理列表里才采信转发头，转发头从右往左找第一个非代理地址。
type ClientIPResolver struct {
	nets []*net.IPNet
}

func NewClientIPResolver(trusted []string) *ClientIPResolver {
	nets := make([]*net.IPNet, 0, len(trusted))
	for _, entry := range trusted {
		if ip := net.ParseIP(entry); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, block, err := net.ParseCIDR(entry); err == nil {
			nets = append(nets, block)
		}
	}
	return &ClientIPResolver{nets: nets}
}

func (r *ClientIPResolver) Of(req *http.Request) string {
	remote := strings.TrimSpace(req.RemoteAddr)
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}

	ip := net.ParseIP(remote)
	if ip == nil {
		return ""
	}
	if !r.trusted(ip) {
		return ip.String()
	}

	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if value := r.forwarded(req.Header.Values(header)); value != "" {
			return value
		}
	}
	return ip.String()
}

func (r *ClientIPResolver) forwarded(values []string) string {
	for _, raw := range values {
		items := strings.Split(raw, ",")
		for i := len(items) - 1; i >= 0; i-- {
			value := strings.TrimSpace(items[i])
			ip := net.ParseIP(value)
			if ip == nil {
				break
			}
			if i == 0 || !r.trusted(ip) {
				return value
			}
		}
	}
	return ""
}

func (r *ClientIPResolver) trusted(ip net.IP) bool {
	for _, block := range r.nets {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
