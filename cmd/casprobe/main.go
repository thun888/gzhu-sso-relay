package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	"github.com/thun888/gzhu-sso-relay/internal/upstream/cas"
	"github.com/thun888/gzhu-sso-relay/internal/upstream/portal"
)

func main() {
	username := os.Getenv("GZHU_USERNAME")
	password := os.Getenv("GZHU_PASSWORD")
	if username == "" || password == "" {
		fmt.Fprintln(os.Stderr, "需要设置 GZHU_USERNAME 与 GZHU_PASSWORD")
		os.Exit(2)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		fail(err)
	}
	httpClient := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fmt.Fprintln(os.Stderr, "-> CAS 登录")
	if err := cas.NewClient(httpClient).Login(ctx, username, password); err != nil {
		fail(err)
	}

	fmt.Fprintln(os.Stderr, "-> 门户会话")
	if err := portal.NewClient(httpClient).Bootstrap(ctx); err != nil {
		fail(err)
	}

	fmt.Fprintln(os.Stderr, "-> 拉取用户信息")
	profile, err := portal.NewClient(httpClient).Profile(ctx)
	if err != nil {
		fail(err)
	}

	out, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		fail(err)
	}
	fmt.Println(string(out))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
