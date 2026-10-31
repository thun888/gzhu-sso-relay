package oidc

import (
	"net/url"
	"testing"
)

func TestRedirectMatches(t *testing.T) {
	cases := []struct {
		name    string
		allowed string
		target  string
		want    bool
	}{
		{"完全一致", "https://app.example.edu.cn/cb", "https://app.example.edu.cn/cb", true},
		{"scheme 与 host 不区分大小写", "https://app.example.edu.cn/cb", "HTTPS://APP.Example.EDU.CN/cb", true},
		{"路径区分大小写", "https://app.example.edu.cn/cb", "https://app.example.edu.cn/CB", false},
		{"端口不同", "https://app.example.edu.cn:8443/cb", "https://app.example.edu.cn:9443/cb", false},
		{"未写端口视为默认端口", "https://app.example.edu.cn/cb", "https://app.example.edu.cn:443/cb", false},
		{"查询串不同", "https://app.example.edu.cn/cb?a=1", "https://app.example.edu.cn/cb?a=2", false},
		{"后缀冒充", "https://app.example.edu.cn/cb", "https://app.example.edu.cn.evil.com/cb", false},
		{"相似主机", "https://app.example.edu.cn/cb", "https://evil-app.example.edu.cn/cb", false},
		{"路径穿越", "https://app.example.edu.cn/cb", "https://app.example.edu.cn/x/../cb", false},
		{"loopback 忽略端口", "http://127.0.0.1/cb", "http://127.0.0.1:53124/cb", true},
		{"localhost 忽略端口", "http://localhost/cb", "http://localhost:8080/cb", true},
		{"IPv6 loopback 忽略端口", "http://[::1]/cb", "http://[::1]:53124/cb", true},
		{"非 loopback 不允许省略端口", "http://app.example.edu.cn/cb", "http://app.example.edu.cn:8080/cb", false},
		{"自定义 scheme", "desktop-app://callback", "desktop-app://callback", true},
		{"自定义 scheme 不匹配", "desktop-app://callback", "desktop-app://other", false},
		{"协议降级", "https://app.example.edu.cn/cb", "http://app.example.edu.cn/cb", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, err := url.Parse(tc.target)
			if err != nil {
				t.Fatalf("解析目标地址: %v", err)
			}
			if got := redirectMatches(tc.allowed, target); got != tc.want {
				t.Fatalf("redirectMatches(%q, %q) = %v, 期望 %v", tc.allowed, tc.target, got, tc.want)
			}
		})
	}
}

func TestVerifyPKCE(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	grant := &Grant{
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
	}
	if !verifyPKCE(grant, verifier) {
		t.Fatal("S256 challenge 应校验通过")
	}
	if verifyPKCE(grant, verifier+"x") {
		t.Fatal("错误的 verifier 不应通过")
	}
	if verifyPKCE(grant, "") {
		t.Fatal("缺少 verifier 不应通过")
	}
}
