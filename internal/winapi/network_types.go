//go:build windows

// 本文件集中定义网卡枚举、设备状态与缓冲区的 Windows 常量。
package winapi

import "golang.org/x/sys/windows"

// 网卡链表与缓冲区的有界解析参数，来自 IP Helper API 推荐值及安全上限。
const (
	maxAdapterCount       = 512
	initialAdapterBufSize = 15 * 1024
	adapterBufGrowth      = 16 * 1024
	maxAdapterRetries     = 5
)

// NetIfAdminStatusUp/Down 是 NET_IF_ADMIN_STATUS 的管理启用/禁用值。
const (
	NetIfAdminStatusUp   = 1
	NetIfAdminStatusDown = 2
	// hardwareInterfaceFlag 是 MIB_IF_ROW2 的 HardwareInterface 位。
	hardwareInterfaceFlag = 0x01
	// filterInterfaceFlag 表示 NDIS 过滤层，不是独立网络适配器。
	filterInterfaceFlag = 0x02
	// cmProblemDisabled/HardwareDisabled 是设备管理器的软件/硬件禁用问题码。
	cmProblemDisabled         = 22
	cmProblemHardwareDisabled = 29
)

// networkDeviceClass 是系统网卡设备类 GUID，只用于只读枚举。
var networkDeviceClass = windows.GUID{Data1: 0x4d36e972, Data2: 0xe325, Data3: 0x11ce, Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18}}
