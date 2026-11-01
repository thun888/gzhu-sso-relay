#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$root"

out=bin
target=""
clean=0

usage() {
	cat <<'EOF'
用法: ./build.sh [选项]

编译 relay、relayctl、casprobe 三个命令。前端产物不在此处生成。

选项:
  -o, --out DIR          输出目录，默认 bin
  -t, --target GOOS/GOARCH
                         交叉编译目标，默认本机，如 linux/amd64
      --clean            构建前清空输出目录
  -h, --help             显示帮助

示例:
  ./build.sh
  ./build.sh -t linux/amd64 -o dist
EOF
}

while [ $# -gt 0 ]; do
	case $1 in
	-o | --out) out=$2; shift 2 ;;
	-t | --target) target=$2; shift 2 ;;
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

if [ ! -f web/dist/index.html ]; then
	echo "缺少 web/dist/index.html，先执行 npm --prefix web run build" >&2
	exit 1
fi

if [ "$clean" = 1 ]; then
	rm -rf "$out"
fi
mkdir -p "$out"

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

commit=$(git rev-parse --short HEAD 2>/dev/null || true)
ldflags="-s -w"
if [ -n "$commit" ]; then
	ldflags="$ldflags -X github.com/thun888/gzhu-sso-relay/internal/build.Commit=$commit"
fi

echo "构建 $goos/$goarch (commit ${commit:-dev}) -> $out"
for name in relay relayctl casprobe; do
	go build -trimpath -ldflags "$ldflags" -o "$out/$name$ext" "./cmd/$name"
	echo "  $out/$name$ext"
done
