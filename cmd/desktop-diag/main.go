//go:build windows

// Command desktop-diag 是 Desktop-Diag 桌面运维一键诊断工具的进程入口。
//
// 本文件只做进程级装配：把命令行、标准输出、标准错误交给编排层 internal/app，
// 再把编排层返回的退出码交给 os.Exit。所有业务逻辑都在 internal/ 各包内。
package main

import (
	"os"

	"github.com/yukin371/desktop-diag/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))
}
