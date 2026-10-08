//go:build windows

// Defines Windows mandatory integrity RID boundaries for read-only token queries.
package winapi

// Integrity levels are security token boundaries defined by Windows.
const (
	integrityLow       uint32 = 0x1000
	integrityMedium    uint32 = 0x2000
	integrityHigh      uint32 = 0x3000
	integritySystem    uint32 = 0x4000
	integrityProtected uint32 = 0x5000
)
