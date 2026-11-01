package build

// Commit 为构建时的短提交号，由 build.sh 通过 -ldflags -X 注入；直接 go build 时保持 dev。
var Commit = "dev"
