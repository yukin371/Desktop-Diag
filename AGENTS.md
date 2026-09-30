# Desktop-Diag

## 项目描述

Desktop-Diag 是一个用于 Windows 平台的桌面诊断工具，旨在收集和分析系统信息，以帮助用户和开发者快速定位和解决问题。该工具通过调用 Win32 API 获取系统硬件、软件和网络等相关信息，并生成详细的诊断报告。

当前本产品应该生成只自己的报告文件 + stdout/stderr。

## 注释规范

**原则：注释应该按照以下原则进行编写：**
- 清晰：清晰描述代码的功能、逻辑和目的，便于理解
- 准确：注释内容应该与代码一致
- 简洁：避免冗长的语句或过多的详细描述
- 有用：提供有关代码的关键信息，如参数和返回值的说明、重要变量的解释等
- 及时更新：当代码发生变化时，应该及时更新

在开头说明文件的功能和用途，并在每个函数和变量前添加注释，说明其功能和用途。
可使用装饰注释分割代码片段便于阅读，减少TODO、FIXME注释使用（应及时完善代码而非注释）

## 代码规范

- 遵循 Effective Go 与 Clean Code：函数短小、单一职责、命名自解释。
- 魔法数字一律提为具名常量，集中在 `internal/detect/thresholds.go`（阈值）与各 `_types.go`（Win32 常量）。
- 错误必须包装上下文：`fmt.Errorf("读取网卡 %s 失败: %w", name, err)`，用 `%w` 保留 `errors.Is` 链。
- 不得出现 `panic`（Leaf 包除外）、不得吞掉 error、不得用 `_ =` 忽略 error（除文档化的例外）。
- 只有直接调用 Win32 API 的包（`winapi`/`collect`/`probe`/`report`）需要 `//go:build windows`；`model`、`detect` 是纯逻辑包，**故意不加**该 tag，以便脱离真机跨平台单测。
- 源文件一律 LF 换行。批量改写源码不要用 PowerShell 的 `WriteAllLines`（它写 CRLF，`gofmt -l` 立刻报错），改用 `WriteAllText` 并自行把 `\r\n` 归一到 `\n`。
- `gofmt` 无差异、`go vet ./...` 零问题、测试通过，才算完成。
