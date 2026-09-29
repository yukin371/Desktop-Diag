// Command desktop-diag 是 Desktop-Diag 桌面运维一键诊断工具的进程入口。
//
// 本文件只负责进程级装配：把 os.Args / stdout / stderr 交给编排层 internal/app，
// 再把编排层返回的退出码交给 os.Exit。所有业务逻辑均在 internal/ 各包内。
//
// 退出码语义（详见 doc/phase0/01-requirements-baseline.md 第 9 节）：
//
//	0  诊断完成，未发现严重告警
//	1  诊断完成，存在严重告警
//	2  程序自身错误（参数非法 / 报告无法生成）
package main

import (
	"fmt"

	"github.com/yukin371/desktop-diag/internal/version"
)

func main() {
	// 阶段 3 的 T-26 会把这里替换为正式编排层调用：
	//     os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))
	// 当前为 T-01 骨架桩，仅用于验证构建与版本注入链路。
	fmt.Println(version.String())
}
