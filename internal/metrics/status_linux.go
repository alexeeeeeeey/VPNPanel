package metrics

import (
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func networkCounters(active []net.Interface) map[string][2]uint64 {
	result := make(map[string][2]uint64)
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, data, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		fields := strings.Fields(data)
		if len(fields) < 16 {
			continue
		}
		for _, iface := range active {
			if iface.Name != name {
				continue
			}
			rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
			tx, txErr := strconv.ParseUint(fields[8], 10, 64)
			if rxErr == nil && txErr == nil {
				result[name] = [2]uint64{rx, tx}
			}
			break
		}
	}
	return result
}

func diskMetrics(path string) (free, total uint64) {
	var stat syscall.Statfs_t
	if syscall.Statfs(path, &stat) == nil {
		return stat.Bavail * uint64(stat.Bsize), stat.Blocks * uint64(stat.Bsize)
	}
	return
}

func hostMetrics() (idle, total, available, memory uint64) {
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		line, _, _ := strings.Cut(string(data), "\n")
		fields := strings.Fields(line)
		if len(fields) >= 9 && fields[0] == "cpu" {
			for i, field := range fields[1:9] {
				value, _ := strconv.ParseUint(field, 10, 64)
				total += value
				if i == 3 || i == 4 {
					idle += value
				}
			}
		}
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			switch fields[0] {
			case "MemTotal:":
				memory, _ = strconv.ParseUint(fields[1], 10, 64)
			case "MemAvailable:":
				available, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
	}
	if available == 0 {
		memory = 0
	}
	return idle, total, available * 1024, memory * 1024
}
