//go:build windows

// Reads registry values using query-only access and contextual errors.
package winapi

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// 本文件封装**只读**注册表访问，只用 OpenKey / GetValue / ReadSubkeyNames 这类读取调用。
// 用 x/sys/windows/registry 而不是手写 advapi32 声明：它已正确处理各值类型的编码差异与缓冲区重试。
// 打开键时只申请真正需要的权限位（不含 NOTIFY），权限位越少越不容易被组策略或安全软件拦截。

// regReadAccess 是读取单个值所需的权限。
const regReadAccess = registry.QUERY_VALUE

// regEnumAccess 是枚举子键所需的权限。
const regEnumAccess = registry.QUERY_VALUE | registry.ENUMERATE_SUB_KEYS

// RegReadString 读取一个字符串值（REG_SZ 或 REG_EXPAND_SZ）。
// REG_EXPAND_SZ 不会被展开：环境变量引用（如 %SystemRoot%）由调用方按需自行处理。
func RegReadString(root registry.Key, path, name string) (string, error) {
	return regReadString(root, path, name, regReadAccess)
}

// RegReadString64 同 RegReadString，但强制使用 64 位注册表视图。
// CurrentVersion 这类键在 64 位系统上有 32/64 两个视图，不显式指定会让不同位数的宿主进程读到不同的 OS 名称。
func RegReadString64(root registry.Key, path, name string) (string, error) {
	return regReadString(root, path, name, regReadAccess|registry.WOW64_64KEY)
}

// regReadString reads a registry string while retaining missing-value and permission distinctions.
func regReadString(root registry.Key, path, name string, access uint32) (string, error) {
	k, err := registry.OpenKey(root, path, access)
	if err != nil {
		return "", wrapRegError("打开", path, err)
	}
	defer k.Close()

	val, _, err := k.GetStringValue(name)
	if err != nil {
		return "", wrapRegErrorValue("读取", path, name, err)
	}
	return val, nil
}

// RegReadUint32 读取一个 REG_DWORD 值。
func RegReadUint32(root registry.Key, path, name string) (uint32, error) {
	k, err := registry.OpenKey(root, path, regReadAccess)
	if err != nil {
		return 0, wrapRegError("打开", path, err)
	}
	defer k.Close()

	val, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, wrapRegErrorValue("读取", path, name, err)
	}
	return uint32(val), nil
}

// RegReadMultiString 读取一个 REG_MULTI_SZ 值。
// 返回的切片已剔除空串：该类型写入时常常留下尾部空元素，透传会让报告出现空行、也会让"DNS 列表为空"的判断失准。
func RegReadMultiString(root registry.Key, path, name string) ([]string, error) {
	k, err := registry.OpenKey(root, path, regReadAccess)
	if err != nil {
		return nil, wrapRegError("打开", path, err)
	}
	defer k.Close()

	vals, _, err := k.GetStringsValue(name)
	if err != nil {
		return nil, wrapRegErrorValue("读取", path, name, err)
	}
	return dropEmptyStrings(vals), nil
}

// RegEnumSubKeys 返回指定路径下的全部子键名。
func RegEnumSubKeys(root registry.Key, path string) ([]string, error) {
	k, err := registry.OpenKey(root, path, regEnumAccess)
	if err != nil {
		return nil, wrapRegError("打开", path, err)
	}
	defer k.Close()

	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, wrapRegError("枚举子键", path, err)
	}
	return names, nil
}

// RegKeyExists 报告指定路径是否存在且可读。
// 用于区分"键不存在"（例如从未配置过该网卡的 DNS，属正常）与"键存在但读不到"（权限不足或被拦截，必须记入 CollectFailure），
// 避免把后者误报成"DNS 配置为空"。
func RegKeyExists(root registry.Key, path string) bool {
	k, err := registry.OpenKey(root, path, regReadAccess)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

// RegReadFirstString 按顺序尝试多个值名，返回第一个存在且非空的值；全部不存在时返回 ErrRegNotFound。
// 用于 DNS 的 NameServer / DhcpNameServer 兜底链：静态配置优先，但静态值为空串时应继续试下一个，而不是认定"无 DNS"。
func RegReadFirstString(root registry.Key, path string, names ...string) (string, string, error) {
	if len(names) == 0 {
		return "", "", fmt.Errorf("读取 %s: 未提供任何候选值名", path)
	}

	k, err := registry.OpenKey(root, path, regReadAccess)
	if err != nil {
		return "", "", wrapRegError("打开", path, err)
	}
	defer k.Close()

	var firstErr error
	for _, name := range names {
		val, _, err := k.GetStringValue(name)
		if err != nil {
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			if firstErr == nil {
				firstErr = wrapRegErrorValue("读取", path, name, err)
			}
			continue
		}
		if trimmed := strings.TrimSpace(val); trimmed != "" {
			return name, trimmed, nil
		}
	}
	if firstErr != nil {
		return "", "", firstErr
	}
	return "", "", fmt.Errorf("读取 %s: %w", path, ErrRegNotFound)
}

// ErrRegNotFound 表示注册表路径或值名不存在。
// 调用方可用 errors.Is(err, ErrRegNotFound) 把"从未配置"（通常正常）与"读取失败"（必须计入诊断完整性告警）分开。
var ErrRegNotFound = errors.New("注册表项不存在")

// wrapRegError 把 registry 的错误附上路径上下文，并在"不存在"时同时挂上 ErrRegNotFound 哨兵。
// 这里用了两个 %w（Go 1.20 起支持），调用方既能匹配 registry.ErrNotExist，也能匹配 ErrRegNotFound。
func wrapRegError(action, path string, err error) error {
	return regWrap(fmt.Sprintf("%s注册表键 %s", action, path), err)
}

// wrapRegErrorValue 同上，但额外带上值名。
func wrapRegErrorValue(action, path, name string, err error) error {
	return regWrap(fmt.Sprintf("%s注册表值 %s\\%s", action, path, name), err)
}

// regWrap adds registry key/value context while preserving the original error chain.
func regWrap(prefix string, err error) error {
	if errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("%s: %w（%w）", prefix, err, ErrRegNotFound)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// dropEmptyStrings 返回剔除空串（含纯空白）后的新切片。
func dropEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// 本工具只读访问的注册表路径。

const (
	// RegPathWindowsVersion 存放操作系统显示名称与版本。
	RegPathWindowsVersion = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`

	// RegPathTcpipInterfaces 每个网卡一个子键（子键名是网卡的 {GUID}）。
	RegPathTcpipInterfaces = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`
)

// Windows 版本键下的值名。
const (
	RegValueProductName    = "ProductName"
	RegValueDisplayVersion = "DisplayVersion"
	RegValueCurrentBuild   = "CurrentBuild"
	RegValueUBR            = "UBR" // 修订号，需与 CurrentBuild 拼成 26200.xxxx
)

// 网卡接口键下的值名。
const (
	RegValueNameServer     = "NameServer"     // 静态配置的 DNS，逗号或空格分隔
	RegValueDhcpNameServer = "DhcpNameServer" // DHCP 下发的 DNS
	RegValueEnableDHCP     = "EnableDHCP"     // 1 = 启用 DHCP
	RegValueDhcpIPAddress  = "DhcpIPAddress"
)

// ParseNameServerList 把注册表里的 DNS 列表文本拆成地址切片，逗号、分号与空白分隔都要支持。
func ParseNameServerList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	return dropEmptyStrings(fields)
}
