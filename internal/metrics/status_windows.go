package metrics

import (
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getIfEntry = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetIfEntry")

// MIB_IFROW contains 32-bit byte counters on Windows.
type interfaceRow struct {
	name                                                                 [256]uint16
	index, kind, mtu, speed, addressLength                               uint32
	address                                                              [8]byte
	admin, operational, lastChange                                       uint32
	inBytes, inUnicast, inMulticast, inDiscards, inErrors, inUnknown     uint32
	outBytes, outUnicast, outMulticast, outDiscards, outErrors, outQueue uint32
	descriptionLength                                                    uint32
	description                                                          [256]byte
}

func networkCounters(active []net.Interface) map[string][2]uint64 {
	result := make(map[string][2]uint64)
	for _, iface := range active {
		row := interfaceRow{index: uint32(iface.Index)}
		if code, _, _ := getIfEntry.Call(uintptr(unsafe.Pointer(&row))); code == 0 {
			result[iface.Name] = [2]uint64{uint64(row.inBytes), uint64(row.outBytes)}
		}
	}
	return result
}

func diskMetrics(path string) (free, total uint64) {
	dir, err := windows.UTF16PtrFromString(path)
	if err == nil {
		_ = windows.GetDiskFreeSpaceEx(dir, &free, &total, nil)
	}
	return
}

var kernel = windows.NewLazySystemDLL("kernel32.dll")
var getSystemTimes = kernel.NewProc("GetSystemTimes")
var getMemoryStatus = kernel.NewProc("GlobalMemoryStatusEx")

type memoryStatus struct {
	length, load                                                                                  uint32
	total, available, totalPage, availablePage, totalVirtual, availableVirtual, availableExtended uint64
}

func hostMetrics() (idle, total, available, memory uint64) {
	var idleTime, kernelTime, userTime uint64
	if ok, _, _ := getSystemTimes.Call(uintptr(unsafe.Pointer(&idleTime)), uintptr(unsafe.Pointer(&kernelTime)), uintptr(unsafe.Pointer(&userTime))); ok != 0 {
		idle, total = idleTime, kernelTime+userTime
	}
	var status memoryStatus
	status.length = uint32(unsafe.Sizeof(status))
	if ok, _, _ := getMemoryStatus.Call(uintptr(unsafe.Pointer(&status))); ok != 0 {
		available, memory = status.available, status.total
	}
	return
}
