//go:build windows

// Collects host identity, Windows version and current token elevation.
package collect

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// win11FirstBuild 是 Windows 11 的首个内部版本号。
//
// Windows 11 的注册表 ProductName 至今仍写作 "Windows 10 ..."（本机 build 26200 实测），
// 照抄注册表会把用户的 Windows 11 报成 Windows 10，而报告是要拿去跟运维对话的证据。
const win11FirstBuild = 22000

// systemCollector 采集主机与操作系统信息。
type systemCollector struct{}

// Name 实现 Collector。
func (systemCollector) Name() string { return "主机与系统信息" }

// EnvVar 实现 envVarer。
func (systemCollector) EnvVar() string { return "system" }

// Collect 实现 Collector。
//
// 逐项尽力而为：任何单项失败都只降级该项，不放弃整台机器的诊断——一台读不到
// 注册表的机器，它的网卡和磁盘数据对运维一样有价值。
func (c systemCollector) Collect(ctx context.Context, snap *model.Snapshot) error {
	var partial []string

	host := &snap.Host

	if name, err := winapi.GetComputerNameEx(winapi.ComputerNameDNSHostname); err != nil {
		partial = append(partial, "计算机名: "+err.Error())
	} else {
		host.ComputerName = name
		snap.AddRaw("主机与系统", "GetComputerNameExW(ComputerNameDnsHostname)", name)
	}

	if u, err := user.Current(); err != nil {
		partial = append(partial, "运行账户: "+err.Error())
	} else {
		host.UserName = u.Username
		snap.AddRaw("主机与系统", "os/user.Current()", u.Username)
	}

	// 只判断，不提升：本工具免管理员运行是硬性需求（REQ-F-705）。
	host.IsAdmin = winapi.IsElevated()
	snap.AddRaw("主机与系统", "Token.IsElevated", fmt.Sprintf("IsAdmin=%v", host.IsAdmin))

	if level, rid, err := winapi.ProcessIntegrity(); err != nil {
		partial = append(partial, "进程完整性等级: "+err.Error())
	} else {
		host.IntegrityLevel, host.IntegrityRID = level, rid
		snap.AddRaw("主机与系统", "GetTokenInformation(TokenIntegrityLevel)", fmt.Sprintf("%s RID=%d", level, rid))
	}

	// 用 RtlGetVersion 而不是 GetVersionEx：后者在未声明兼容性的清单下会谎报版本。
	if v, err := winapi.RtlGetVersion(); err != nil {
		partial = append(partial, "操作系统版本: "+err.Error())
	} else {
		host.OSBuild = fmt.Sprintf("%d", v.BuildNumber)
		snap.AddRaw("主机与系统", "ntdll!RtlGetVersion",
			fmt.Sprintf("Version=%d.%d Build=%d PlatformId=%d",
				v.MajorVersion, v.MinorVersion, v.BuildNumber, v.PlatformID))
		host.OSVersion = formatOSVersion(v.MajorVersion, v.MinorVersion, v.BuildNumber, 0)

		// 磁盘/内存的结论不依赖它，但它是报告头的第一行，值得单独读注册表。
		productName, ubr, regErr := readOSRegistry(snap)
		if regErr != nil {
			partial = append(partial, "操作系统显示名: "+regErr.Error())
		} else {
			host.OSName = normalizeOSName(productName, v.BuildNumber)
			host.OSVersion = formatOSVersion(v.MajorVersion, v.MinorVersion, v.BuildNumber, ubr)
		}
	}

	host.OSArch = archDisplay(runtime.GOARCH)
	snap.AddRaw("主机与系统", "runtime.GOARCH", runtime.GOARCH)

	if ms, err := winapi.GetTickCount64(); err != nil {
		partial = append(partial, "系统运行时长: "+err.Error())
	} else {
		// GetTickCount64 的语义是"系统已运行毫秒数"，不含休眠时间，因此反推的开机
		// 时刻在休眠过的机器上会偏晚；该值只用于表达"运行了多久"，不作为开机时刻的证据。
		host.Uptime = time.Duration(ms) * time.Millisecond
		host.BootTime = snap.StartedAt.Add(-host.Uptime)
		snap.AddRaw("主机与系统", "kernel32!GetTickCount64",
			fmt.Sprintf("%d ms → 运行 %s", ms, host.Uptime.Round(time.Second)))
	}

	if len(partial) > 0 {
		// 部分采集：报告第二层会逐条标注"因 X 未采集"。
		snap.AddFailure(c.Name(), c.EnvVar(), strings.Join(partial, "；"), true)
	}
	if host.ComputerName == "" && host.OSVersion == "" {
		return errors.New("主机与系统信息完全无法采集")
	}
	return nil
}

// readOSRegistry 读取操作系统显示名与修订号（UBR）。
//
// 两个值分别读：任一缺失都不应让另一个也拿不到。
func readOSRegistry(snap *model.Snapshot) (productName string, ubr uint32, err error) {
	var problems []string

	// 显式用 64 位视图：CurrentVersion 键在 32/64 位视图下内容不同，
	// 不指定视图会让同一条命令在不同宿主下读出不同的系统名。
	if name, e := winapi.RegReadString64(registry.LOCAL_MACHINE, winapi.RegPathWindowsVersion, winapi.RegValueProductName); e != nil {
		problems = append(problems, "ProductName: "+e.Error())
	} else {
		productName = name
		snap.AddRaw("主机与系统", `HKLM\`+winapi.RegPathWindowsVersion+`\ProductName`, name)
	}

	if n, e := winapi.RegReadUint32(registry.LOCAL_MACHINE, winapi.RegPathWindowsVersion, winapi.RegValueUBR); e != nil {
		// UBR 缺失不算问题：老系统上确实可能没有这个值，版本号少一段不影响结论。
		snap.AddRaw("主机与系统", `HKLM\`+winapi.RegPathWindowsVersion+`\UBR`, "（无此值）"+e.Error())
	} else {
		ubr = n
		snap.AddRaw("主机与系统", `HKLM\`+winapi.RegPathWindowsVersion+`\UBR`, fmt.Sprintf("%d", n))
	}

	// DisplayVersion（如 "25H2"）只作为附录信息，不进结论。
	if dv, e := winapi.RegReadString64(registry.LOCAL_MACHINE, winapi.RegPathWindowsVersion, winapi.RegValueDisplayVersion); e == nil {
		snap.AddRaw("主机与系统", `HKLM\`+winapi.RegPathWindowsVersion+`\DisplayVersion`, dv)
	}

	if len(problems) > 0 {
		return "", 0, errors.New(strings.Join(problems, "；"))
	}
	return productName, ubr, nil
}

// normalizeOSName 把注册表里的 ProductName 纠正为可对外陈述的系统名。
//
// 只改写以 "Windows 10" 开头且 build >= 22000 的名称：Windows Server 系列
// （ProductName 是 "Windows Server 2022/2025"）绝不能被误改成客户端系统名。
func normalizeOSName(productName string, buildNumber uint32) string {
	name := strings.TrimSpace(productName)
	if name == "" {
		return "未知（注册表 ProductName 为空）"
	}
	if buildNumber >= win11FirstBuild && strings.HasPrefix(name, "Windows 10") {
		return "Windows 11" + strings.TrimPrefix(name, "Windows 10")
	}
	return name
}

// formatOSVersion 拼出 "10.0.26200.9457"。
//
// ubr 为 0 时省略，避免把"读不到修订号"写成 ".0" 让人误以为修订号就是 0。
func formatOSVersion(major, minor, build, ubr uint32) string {
	v := fmt.Sprintf("%d.%d.%d", major, minor, build)
	if ubr > 0 {
		v += fmt.Sprintf(".%d", ubr)
	}
	return v
}

// archDisplay 把 Go 的架构名转成运维人员看得懂的说法。
func archDisplay(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86-64 (64 位)"
	case "arm64":
		return "ARM64 (64 位)"
	case "386":
		return "x86 (32 位)"
	default:
		return goarch
	}
}
