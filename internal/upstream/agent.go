package upstream

import "strings"

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

const maxUserAgentLength = 512

func RequestUserAgent(raw string) string {
	ua := strings.TrimSpace(raw)
	if ua == "" || len(ua) > maxUserAgentLength || strings.ContainsFunc(ua, invalidHeaderRune) {
		return DefaultUserAgent
	}
	return ua
}

func invalidHeaderRune(r rune) bool {
	return r < 0x20 || r == 0x7f
}
