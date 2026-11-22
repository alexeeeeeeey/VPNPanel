//go:build !linux && !windows

package metrics

import "net"

func hostMetrics() (idle, total, available, memory uint64) { return }
func networkCounters([]net.Interface) map[string][2]uint64 { return nil }
func diskMetrics(string) (free, total uint64)              { return }
