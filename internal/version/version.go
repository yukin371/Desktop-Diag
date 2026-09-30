// Package version 提供构建期注入的版本信息。
//
// Version、Commit、BuildTime 三个变量由构建脚本通过
// -ldflags "-X github.com/yukin371/desktop-diag/internal/version.Version=..." 注入。
// 未注入时（例如 go run、go test、IDE 直接调试）保留占位默认值，
// 保证任何情况下调用 String() 都不会返回空串。
package version

import (
	"fmt"
	"runtime"
)

// 构建期注入的变量。默认值用于未经构建脚本的场景。
var (
	// Version 语义化版本号，例如 "v0.1.0"。
	Version = "dev"
	// Commit 构建时的 git 提交短哈希。
	Commit = "unknown"
	// BuildTime 构建时间，UTC，RFC3339 格式。
	BuildTime = "unknown"
)

// String 返回单行版本描述，供控制台与报告头使用。
func String() string {
	return fmt.Sprintf("Desktop-Diag %s（提交 %s，构建时间 %s，%s/%s，%s）",
		Version, Commit, BuildTime, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// Short 返回简短版本号，供报告头等空间受限处使用。
func Short() string {
	return Version
}
