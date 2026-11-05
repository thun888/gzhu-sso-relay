package web

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

const configScript = "window.__RELAY_CONFIG__="

// Handler 提供前端静态资源，未命中的路径回落到 index.html 以支持前端路由。
// inline 为页面返回配置 JSON，nil 表示不注入。
func Handler(inline func(*http.Request) []byte) (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, fmt.Errorf("web: 定位前端产物: %w", err)
	}

	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, fmt.Errorf("web: 前端产物缺失，先执行 npm --prefix web run build: %w", err)
	}

	files := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(sub, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(r.URL.Path, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
		}

		page := index
		if inline != nil {
			if payload := inline(r); len(payload) > 0 {
				page = injectConfig(index, payload)
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "private, no-cache")
		_, _ = w.Write(page)
	}), nil
}

// injectConfig 把配置写成内联脚本插到 head 结束前，页面首帧即可直接读取。
func injectConfig(index, payload []byte) []byte {
	script := make([]byte, 0, len(payload)+len(configScript)+len("</script>")+len("<script>"))
	script = append(script, "<script>"...)
	script = append(script, configScript...)
	script = append(script, payload...)
	script = append(script, "</script>"...)

	if pos := bytes.Index(index, []byte("</head>")); pos >= 0 {
		out := make([]byte, 0, len(index)+len(script))
		out = append(out, index[:pos]...)
		out = append(out, script...)
		return append(out, index[pos:]...)
	}

	out := make([]byte, 0, len(index)+len(script))
	out = append(out, index...)
	return append(out, script...)
}
