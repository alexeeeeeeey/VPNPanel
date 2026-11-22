package metrics

import (
	"context"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"vpnpanel/internal/buildinfo"
	"vpnpanel/internal/store"
)

type networkInterface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
}

func interfaces() ([]networkInterface, []net.Interface) {
	list, _ := net.Interfaces()
	result := []networkInterface{}
	active := []net.Interface{}
	for _, iface := range list {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		active = append(active, iface)
		addresses, _ := iface.Addrs()
		row := networkInterface{Name: iface.Name}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && !ip.IsUnspecified() {
				row.Addresses = append(row.Addresses, ip.String())
			}
		}
		if len(row.Addresses) > 0 {
			result = append(result, row)
		}
	}
	return result, active
}

type metricSample struct {
	SampledAt float64  `json:"sampled_at"`
	CPU       *float64 `json:"cpu"`
	Memory    *float64 `json:"memory"`
	RX        *float64 `json:"rx"`
	TX        *float64 `json:"tx"`
}

func NewSampler(ctx context.Context, storagePath string, s *store.Store) func() map[string]any {
	started := time.Now()
	var mu sync.Mutex
	var previousIdle, previousTotal uint64
	var hasPrevious bool
	var previousNetwork map[string][2]uint64
	var previousSample time.Time
	var latest map[string]any
	history := make([]metricSample, 0, 76)
	name, _ := os.Hostname()
	collect := func() {
		identity, _ := s.Identity()
		addresses, active := interfaces()
		counters := networkCounters(active)
		now := time.Now()
		var rx, tx *float64
		if elapsed := now.Sub(previousSample).Seconds(); previousNetwork != nil && elapsed > 0 {
			for name, current := range counters {
				if previous, ok := previousNetwork[name]; ok && current[0] >= previous[0] && current[1] >= previous[1] {
					if rx == nil {
						rx, tx = new(float64), new(float64)
					}
					*rx += float64(current[0]-previous[0]) / elapsed
					*tx += float64(current[1]-previous[1]) / elapsed
				}
			}
		}
		previousNetwork, previousSample = counters, now
		idle, total, available, memory := hostMetrics()
		var cpu, ram *float64
		if hasPrevious && total > previousTotal && idle >= previousIdle && idle-previousIdle <= total-previousTotal {
			value := 100 * (1 - float64(idle-previousIdle)/float64(total-previousTotal))
			cpu = &value
		}
		previousIdle, previousTotal, hasPrevious = idle, total, total > 0
		diskFree, diskTotal := diskMetrics(storagePath)
		if memory > 0 && available <= memory {
			value := 100 * float64(memory-available) / float64(memory)
			ram = &value
		}
		sample := metricSample{float64(now.UnixMilli()) / 1000, cpu, ram, rx, tx}
		mu.Lock()
		defer mu.Unlock()
		cutoff := sample.SampledAt - 300
		first := 0
		for first < len(history) && history[first].SampledAt < cutoff {
			first++
		}
		if first > 0 {
			history = history[:copy(history, history[first:])]
		}
		history = append(history, sample)
		latest = map[string]any{
			"node_id": identity.ID, "node_name": identity.Name,
			"hostname": name, "os": runtime.GOOS, "cores": runtime.NumCPU(),
			"version": buildinfo.Version, "uptime": int64(time.Since(started).Seconds()),
			"cpu": cpu, "memory": ram, "memory_total": memory,
			"interfaces": addresses, "disk_free": diskFree, "disk_total": diskTotal,
			"rx": rx, "tx": tx, "sampled_at": sample.SampledAt,
		}
	}
	collect()
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collect()
			}
		}
	}()
	return func() map[string]any {
		mu.Lock()
		result := make(map[string]any, len(latest)+1)
		for key, value := range latest {
			result[key] = value
		}
		result["history"] = append([]metricSample{}, history...)
		mu.Unlock()
		return result
	}
}
