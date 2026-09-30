//go:build windows

// 本文件只读枚举接口管理状态及网络栈未返回的禁用设备，不改变网卡配置。
package winapi

import (
	"errors"
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// GetInterfaceInventory 使用 x/sys 的 SDK 布局枚举全部接口，不手写 MIB_IF_ROW2。
func GetInterfaceInventory() ([]RawAdapter, error) {
	// table 由系统分配，只能使用 FreeMibTable 释放。
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormalWithoutStatistics, &table); err != nil {
		return nil, fmt.Errorf("读取网卡接口管理状态失败: %w", err)
	}
	if table == nil {
		return nil, errors.New("读取网卡接口管理状态失败：返回空表")
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	if table.NumEntries > maxAdapterCount {
		return nil, fmt.Errorf("网卡接口表条目数 %d 超出安全上限 %d", table.NumEntries, maxAdapterCount)
	}
	// rows 是系统返回的数组视图，仅在 table 释放前使用。
	rows := unsafe.Slice(&table.Table[0], int(table.NumEntries))
	// result 拷贝全部字符串及状态，避免返回悬空的系统内存引用。
	result := make([]RawAdapter, 0, len(rows))
	for _, row := range rows {
		// 不把 QoS/WFP/安全软件过滤层计为额外设备。
		if row.InterfaceAndOperStatusFlags&filterInterfaceFlag != 0 {
			continue
		}
		// adapter 只包含接口表能证明的元信息，地址仍由 GetAdaptersAddresses 提供。
		adapter := RawAdapter{IfIndex: row.InterfaceIndex, AdapterName: row.InterfaceGuid.String(), FriendlyName: windows.UTF16ToString(row.Alias[:]), Description: windows.UTF16ToString(row.Description[:]), IfType: row.Type, OperStatus: row.OperStatus, AdminKnown: row.AdminStatus == NetIfAdminStatusUp || row.AdminStatus == NetIfAdminStatusDown, AdminEnabled: row.AdminStatus == NetIfAdminStatusUp, HardwareKnown: true, HardwareInterface: row.InterfaceAndOperStatusFlags&hardwareInterfaceFlag != 0}
		if row.PhysicalAddressLength > uint32(len(row.PhysicalAddress)) {
			return result, fmt.Errorf("接口 %d 的 MAC 长度 %d 非法", row.InterfaceIndex, row.PhysicalAddressLength)
		}
		adapter.PhysicalAddress = net.HardwareAddr(row.PhysicalAddress[:row.PhysicalAddressLength]).String()
		result = append(result, adapter)
	}
	return result, nil
}

// GetDisabledNetworkDevices 补充因设备禁用而未出现在接口表中的当前网卡。
func GetDisabledNetworkDevices() (result []RawAdapter, err error) {
	// devices 只建立当前设备的内存枚举集合，不创建设备或修改配置。
	devices, err := windows.SetupDiGetClassDevsEx(&networkDeviceClass, "", 0, windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return nil, fmt.Errorf("只读枚举禁用网卡失败: %w", err)
	}
	defer func() {
		if closeErr := devices.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("释放网卡枚举集合失败: %w", closeErr))
		}
	}()
	for i := 0; ; i++ {
		// device 表示一个已存在的网卡设备节点。
		device, enumErr := devices.EnumDeviceInfo(i)
		if errors.Is(enumErr, windows.ERROR_NO_MORE_ITEMS) {
			return result, err
		}
		if enumErr != nil {
			return result, errors.Join(err, fmt.Errorf("读取网卡设备 %d 失败: %w", i, enumErr))
		}
		// status/problem 是配置管理器只读设备状态，不依据链路 Down 猜测禁用。
		var status, problem uint32
		if statusErr := windows.CM_Get_DevNode_Status(&status, &problem, device.DevInst, 0); statusErr != nil {
			err = errors.Join(err, fmt.Errorf("读取网卡设备 %d 管理状态失败: %w", i, statusErr))
			continue
		}
		if problem != cmProblemDisabled && problem != cmProblemHardwareDisabled {
			continue
		}
		// instance 是设备稳定身份，驱动键不存在时仍可展示禁用设备。
		instance, instanceErr := devices.DeviceInstanceID(device)
		if instanceErr != nil {
			err = errors.Join(err, fmt.Errorf("读取禁用网卡 %d 身份失败: %w", i, instanceErr))
			continue
		}
		// description 来自设备描述属性，读取失败时保留实例名并记录原因。
		description, descriptionErr := devices.DeviceRegistryProperty(device, windows.SPDRP_DEVICEDESC)
		if descriptionErr != nil {
			err = errors.Join(err, fmt.Errorf("读取禁用网卡 %s 描述失败: %w", instance, descriptionErr))
		}
		// adapter 明确禁用与未知地址，不伪造 IPv4、MAC 或接口索引。
		adapter := RawAdapter{AdapterName: instance, FriendlyName: instance, Description: instance, AdminKnown: true, AdminEnabled: false, OperStatus: IfOperStatusDown}
		if value, ok := description.(string); ok {
			adapter.Description = value
			adapter.FriendlyName = value
		}
		// key 只以 QUERY_VALUE 打开驱动配置，读取 NetCfgInstanceId 用于和接口表去重。
		key, keyErr := devices.OpenDevRegKey(device, windows.DICS_FLAG_GLOBAL, 0, windows.DIREG_DRV, registry.QUERY_VALUE)
		if keyErr != nil {
			err = errors.Join(err, fmt.Errorf("读取禁用网卡 %s 的只读驱动键失败: %w", instance, keyErr))
		} else {
			// regKey 借用句柄后负责唯一一次关闭。
			regKey := registry.Key(key)
			id, _, readErr := regKey.GetStringValue("NetCfgInstanceId")
			if readErr == nil {
				adapter.AdapterName = id
			} else {
				err = errors.Join(err, fmt.Errorf("读取禁用网卡 %s 接口 GUID 失败: %w", instance, readErr))
			}
			if closeErr := regKey.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("关闭禁用网卡 %s 的只读驱动键失败: %w", instance, closeErr))
			}
		}
		result = append(result, adapter)
	}
}
