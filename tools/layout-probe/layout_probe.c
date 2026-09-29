// 结构体布局取证程序：用 MSVC 编译真实的 Windows SDK 头文件，
// 打印编译器算出的 sizeof / alignof / 字段偏移。
// 这是 Go 侧断言表（internal/winapi/*_types_test.go）的**权威来源**。
// 用法见 tools/layout-probe/README.md。
#define WIN32_LEAN_AND_MEAN
#include <winsock2.h>
#include <ws2tcpip.h>
#include <iphlpapi.h>
#include <iptypes.h>
#include <ipexport.h>
#include <winternl.h>
#include <stdio.h>
#include <stddef.h>

// 注意：这些类型是 `typedef struct _X X`，所以 offsetof 直接用 typedef 名，
// 写成 `struct X` 会命中未定义的结构体（C2037）。
#define P(s, f) printf("%-28s %-22s %4llu\n", #s, #f, (unsigned long long)offsetof(s, f))
#define PS(s)   printf("%-28s %-22s %4llu\n", #s, "sizeof", (unsigned long long)sizeof(s))
#define PA(s)   printf("%-28s %-22s %4llu\n", #s, "alignof", (unsigned long long)__alignof(s))

int main(void) {
    printf("=== IP_ADAPTER_ADDRESSES_LH ===\n");
    PS(IP_ADAPTER_ADDRESSES_LH);
    PA(IP_ADAPTER_ADDRESSES_LH);
    P(IP_ADAPTER_ADDRESSES_LH, Length);
    P(IP_ADAPTER_ADDRESSES_LH, IfIndex);
    P(IP_ADAPTER_ADDRESSES_LH, Next);
    P(IP_ADAPTER_ADDRESSES_LH, AdapterName);
    P(IP_ADAPTER_ADDRESSES_LH, FirstUnicastAddress);
    P(IP_ADAPTER_ADDRESSES_LH, FirstAnycastAddress);
    P(IP_ADAPTER_ADDRESSES_LH, FirstMulticastAddress);
    P(IP_ADAPTER_ADDRESSES_LH, FirstDnsServerAddress);
    P(IP_ADAPTER_ADDRESSES_LH, DnsSuffix);
    P(IP_ADAPTER_ADDRESSES_LH, Description);
    P(IP_ADAPTER_ADDRESSES_LH, FriendlyName);
    P(IP_ADAPTER_ADDRESSES_LH, PhysicalAddress);
    P(IP_ADAPTER_ADDRESSES_LH, PhysicalAddressLength);
    P(IP_ADAPTER_ADDRESSES_LH, Flags);
    P(IP_ADAPTER_ADDRESSES_LH, Mtu);
    P(IP_ADAPTER_ADDRESSES_LH, IfType);
    P(IP_ADAPTER_ADDRESSES_LH, OperStatus);
    P(IP_ADAPTER_ADDRESSES_LH, Ipv6IfIndex);
    P(IP_ADAPTER_ADDRESSES_LH, ZoneIndices);
    P(IP_ADAPTER_ADDRESSES_LH, FirstPrefix);
    P(IP_ADAPTER_ADDRESSES_LH, TransmitLinkSpeed);
    P(IP_ADAPTER_ADDRESSES_LH, ReceiveLinkSpeed);
    P(IP_ADAPTER_ADDRESSES_LH, FirstWinsServerAddress);
    P(IP_ADAPTER_ADDRESSES_LH, FirstGatewayAddress);
    P(IP_ADAPTER_ADDRESSES_LH, Ipv4Metric);
    P(IP_ADAPTER_ADDRESSES_LH, Ipv6Metric);
    P(IP_ADAPTER_ADDRESSES_LH, Luid);
    P(IP_ADAPTER_ADDRESSES_LH, Dhcpv4Server);
    P(IP_ADAPTER_ADDRESSES_LH, CompartmentId);
    P(IP_ADAPTER_ADDRESSES_LH, NetworkGuid);
    P(IP_ADAPTER_ADDRESSES_LH, ConnectionType);
    P(IP_ADAPTER_ADDRESSES_LH, TunnelType);
    P(IP_ADAPTER_ADDRESSES_LH, Dhcpv6Server);
    P(IP_ADAPTER_ADDRESSES_LH, Dhcpv6ClientDuid);
    P(IP_ADAPTER_ADDRESSES_LH, Dhcpv6ClientDuidLength);
    P(IP_ADAPTER_ADDRESSES_LH, Dhcpv6Iaid);
    P(IP_ADAPTER_ADDRESSES_LH, FirstDnsSuffix);

    printf("\n=== IP_ADAPTER_UNICAST_ADDRESS_LH ===\n");
    PS(IP_ADAPTER_UNICAST_ADDRESS_LH);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, Length);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, Flags);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, Next);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, Address);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, PrefixOrigin);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, SuffixOrigin);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, DadState);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, ValidLifetime);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, PreferredLifetime);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, LeaseLifetime);
    P(IP_ADAPTER_UNICAST_ADDRESS_LH, OnLinkPrefixLength);

    printf("\n=== IP_ADAPTER_DNS_SERVER_ADDRESS_XP ===\n");
    PS(IP_ADAPTER_DNS_SERVER_ADDRESS_XP);
    P(IP_ADAPTER_DNS_SERVER_ADDRESS_XP, Length);
    P(IP_ADAPTER_DNS_SERVER_ADDRESS_XP, Reserved);
    P(IP_ADAPTER_DNS_SERVER_ADDRESS_XP, Next);
    P(IP_ADAPTER_DNS_SERVER_ADDRESS_XP, Address);

    printf("\n=== IP_ADAPTER_GATEWAY_ADDRESS_LH ===\n");
    PS(IP_ADAPTER_GATEWAY_ADDRESS_LH);
    P(IP_ADAPTER_GATEWAY_ADDRESS_LH, Length);
    P(IP_ADAPTER_GATEWAY_ADDRESS_LH, Reserved);
    P(IP_ADAPTER_GATEWAY_ADDRESS_LH, Next);
    P(IP_ADAPTER_GATEWAY_ADDRESS_LH, Address);

    printf("\n=== IP_ADAPTER_WINS_SERVER_ADDRESS_LH ===\n");
    PS(IP_ADAPTER_WINS_SERVER_ADDRESS_LH);
    P(IP_ADAPTER_WINS_SERVER_ADDRESS_LH, Address);

    printf("\n=== sockets ===\n");
    PS(SOCKET_ADDRESS); PA(SOCKET_ADDRESS);
    P(SOCKET_ADDRESS, lpSockaddr);
    P(SOCKET_ADDRESS, iSockaddrLength);
    PS(SOCKADDR_IN); P(SOCKADDR_IN, sin_addr); P(SOCKADDR_IN, sin_family);
    PS(SOCKADDR_IN6); PA(SOCKADDR_IN6);
    P(SOCKADDR_IN6, sin6_addr); P(SOCKADDR_IN6, sin6_scope_id);
    PS(SOCKADDR);

    printf("\n=== ICMP ===\n");
    PS(ICMP_ECHO_REPLY); PA(ICMP_ECHO_REPLY);
    P(ICMP_ECHO_REPLY, Address);
    P(ICMP_ECHO_REPLY, Status);
    P(ICMP_ECHO_REPLY, RoundTripTime);
    P(ICMP_ECHO_REPLY, DataSize);
    P(ICMP_ECHO_REPLY, Reserved);
    P(ICMP_ECHO_REPLY, Data);
    P(ICMP_ECHO_REPLY, Options);
    PS(IP_OPTION_INFORMATION);
    P(IP_OPTION_INFORMATION, Ttl);
    P(IP_OPTION_INFORMATION, OptionsData);
    PS(IF_LUID);
    PS(NET_IF_NETWORK_GUID);

    // 关键区别：RtlGetVersion 按 dwOSVersionInfoSize 判断调用方用的是哪个版本。
    // RTL_OSVERSIONINFOW 是 276 字节；RTL_OSVERSIONINFOEXW 在 szCSDVersion 之后
    // 还多 5 个字段（wServicePackMajor/Minor、wSuiteMask、wProductType、wReserved）。
    // Go 侧只声明到 szCSDVersion，镜像的是**非 EX** 版本。
    printf("\n=== ntdll ===\n");
    PS(RTL_OSVERSIONINFOW); PA(RTL_OSVERSIONINFOW);
    P(RTL_OSVERSIONINFOW, dwOSVersionInfoSize);
    P(RTL_OSVERSIONINFOW, dwMajorVersion);
    P(RTL_OSVERSIONINFOW, dwMinorVersion);
    P(RTL_OSVERSIONINFOW, dwBuildNumber);
    P(RTL_OSVERSIONINFOW, dwPlatformId);
    P(RTL_OSVERSIONINFOW, szCSDVersion);
    PS(RTL_OSVERSIONINFOEXW); PA(RTL_OSVERSIONINFOEXW);
    P(RTL_OSVERSIONINFOEXW, szCSDVersion);
    P(RTL_OSVERSIONINFOEXW, wServicePackMajor);

    printf("\n=== kernel32 ===\n");
    PS(MEMORYSTATUSEX); PA(MEMORYSTATUSEX);
    P(MEMORYSTATUSEX, dwLength);
    P(MEMORYSTATUSEX, dwMemoryLoad);
    P(MEMORYSTATUSEX, ullTotalPhys);
    P(MEMORYSTATUSEX, ullAvailPhys);
    P(MEMORYSTATUSEX, ullTotalPageFile);
    P(MEMORYSTATUSEX, ullAvailPageFile);
    P(MEMORYSTATUSEX, ullTotalVirtual);
    P(MEMORYSTATUSEX, ullAvailVirtual);
    P(MEMORYSTATUSEX, ullAvailExtendedVirtual);

    printf("\n=== constants ===\n");
    printf("MAX_ADAPTER_ADDRESS_LENGTH   %d\n", MAX_ADAPTER_ADDRESS_LENGTH);
    printf("MAX_DHCPV6_DUID_LENGTH       %d\n", MAX_DHCPV6_DUID_LENGTH);
    printf("MAX_ADAPTER_NAME_LENGTH      %d\n", MAX_ADAPTER_NAME_LENGTH);
    printf("MAX_ADAPTER_DESCRIPTION_LENGTH %d\n", MAX_ADAPTER_DESCRIPTION_LENGTH);
    printf("AF_UNSPEC %d AF_INET %d AF_INET6 %d\n", AF_UNSPEC, AF_INET, AF_INET6);
    printf("GAA_FLAG_SKIP_ANYCAST 0x%04X SKIP_MULTICAST 0x%04X SKIP_DNS_SERVER 0x%04X INCLUDE_PREFIX 0x%04X INCLUDE_GATEWAYS 0x%04X\n",
           GAA_FLAG_SKIP_ANYCAST, GAA_FLAG_SKIP_MULTICAST, GAA_FLAG_SKIP_DNS_SERVER,
           GAA_FLAG_INCLUDE_PREFIX, GAA_FLAG_INCLUDE_GATEWAYS);
    printf("IF_TYPE_ETHERNET_CSMACD %d IF_TYPE_PPP %d IF_TYPE_SOFTWARE_LOOPBACK %d IF_TYPE_IEEE80211 %d IF_TYPE_TUNNEL %d\n",
           IF_TYPE_ETHERNET_CSMACD, IF_TYPE_PPP, IF_TYPE_SOFTWARE_LOOPBACK, IF_TYPE_IEEE80211, IF_TYPE_TUNNEL);
    printf("IfOperStatusUp %d Down %d Testing %d Unknown %d Dormant %d NotPresent %d LowerLayerDown %d\n",
           IfOperStatusUp, IfOperStatusDown, IfOperStatusTesting, IfOperStatusUnknown,
           IfOperStatusDormant, IfOperStatusNotPresent, IfOperStatusLowerLayerDown);
    printf("IP_SUCCESS %d DEST_NET_UNREACH %d DEST_HOST_UNREACH %d DEST_PROT_UNREACH %d DEST_PORT_UNREACH %d REQ_TIMED_OUT %d BAD_DESTINATION %d GENERAL_FAILURE %d\n",
           IP_SUCCESS, IP_DEST_NET_UNREACHABLE, IP_DEST_HOST_UNREACHABLE, IP_DEST_PROT_UNREACHABLE,
           IP_DEST_PORT_UNREACHABLE, IP_REQ_TIMED_OUT, IP_BAD_DESTINATION, IP_GENERAL_FAILURE);
    printf("ERROR_BUFFER_OVERFLOW %d ERROR_NO_DATA %d ERROR_ADDRESS_NOT_ASSOCIATED %d\n",
           ERROR_BUFFER_OVERFLOW, ERROR_NO_DATA, ERROR_ADDRESS_NOT_ASSOCIATED);
    printf("Dhcpv4Enabled bit: ");
    {
        IP_ADAPTER_ADDRESSES_LH a;
        memset(&a, 0, sizeof(a));
        a.Dhcpv4Enabled = 1;
        printf("Flags=0x%08X\n", a.Flags);
    }
    return 0;
}
