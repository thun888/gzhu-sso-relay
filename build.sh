#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$root"

out=bin
target=""
build_web=1
clean=0

usage() {
	cat <<'EOF'
用法: ./build.sh [选项]

选项:
  -o, --out DIR          输出目录，默认 bin
  -t, --target GOOS/GOARCH
                         交叉编译目标，默认本机，如 linux/amd64
      --no-web           跳过前端构建，直接使用现有 web/dist
      --clean            构建前清空输出目录
  -h, --help             显示帮助

示例:
  ./build.sh
  ./build.sh -t linux/amd64 -o dist
  ./build.sh --no-web
EOF
}

while [ $# -gt 0 ]; do
	case $1 in
	-o | --out) out=$2; shift 2 ;;
	-t | --target) target=$2; shift 2 ;;
	--no-web) build_web=0; shift ;;
	--clean) clean=1; shift ;;
	-h | --help) usage; exit 0 ;;
	*)
		echo "未知参数: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

command -v go >/dev/null || {
	echo "未找到 go 命令" >&2
	exit 1
}

if [ "$clean" = 1 ]; then
	rm -rf "$out"
fi
mkdir -p "$out"

if [ "$build_web" = 1 ]; then
	command -v npm >/dev/null || {
		echo "未找到 npm 命令，可用 --no-web 复用已有 web/dist" >&2
		exit 1
	}
	if [ ! -d web/node_modules ]; then
		npm --prefix web ci
	fi
	npm --prefix web run build
fi

if [ ! -f web/dist/index.html ]; then
	echo "web/dist 缺少 index.html，go:embed 无法打包前端" >&2
	exit 1
fi

goos=$(go env GOOS)
goarch=$(go env GOARCH)
if [ -n "$target" ]; then
	goos=${target%%/*}
	goarch=${target##*/}
fi

ext=""
if [ "$goos" = windows ]; then
	ext=.exe
fi

export CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch"

echo "构建 $goos/$goarch -> $out"
for name in relay relayctl casprobe; do
	go build -trimpath -ldflags "-s -w" -o "$out/$name$ext" "./cmd/$name"
	echo "  $out/$name$ext"
done
