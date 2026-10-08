// Package buildinfo 保存构建期注入的版本号.
//
// 发布构建通过 -ldflags -X 注入版本号, 例如:
//
//	go build -trimpath -ldflags "-s -w -X $(go list -m)/internal/buildinfo.version=v1.2.3"
//
// 日常开发构建不注入, 显示 dev-build. 版本号的具体形态由
// scripts/build-version.py 决定: 停在 tag 上时就是该 tag, 非 tag commit
// 追加 7 位短 hash, 工作区有未提交改动时改用 ^ 分隔.
package buildinfo

// version 由发布构建通过 -ldflags -X 覆盖, 不要改成常量, 否则链接期无法注入.
var version = "dev-build"

// Version 返回当前构建应当显示的版本号.
func Version() string {
	return version
}
