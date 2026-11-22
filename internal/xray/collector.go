package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"vpnpanel/internal/store"
)

type Collector struct {
	mu          sync.Mutex
	LastPoll    int64  `json:"last_poll"`
	Error       string `json:"error"`
	OnlineError string `json:"online_error"`
}

type UsageStatus struct {
	LastPoll    int64  `json:"last_poll"`
	Error       string `json:"error"`
	OnlineError string `json:"online_error"`
}

func (collector *Collector) Status() UsageStatus {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return UsageStatus{collector.LastPoll, collector.Error, collector.OnlineError}
}
func xrayJSON(ctx context.Context, binary, address, command string, args ...string) ([]byte, error) {
	query, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	argv := append([]string{"api", command, "--server=" + address, "-json"}, args...)
	output, err := exec.CommandContext(query, binary, argv...).Output()
	if err != nil {
		return nil, fmt.Errorf("Xray %s: %w", command, err)
	}
	return output, nil
}
func emailUser(email string) (string, string, bool) {
	id, credential, found := strings.Cut(email, ".")
	return id, credential, found && (credential == "main" || credential == "limited")
}
func counterValue(raw json.RawMessage) (int64, error) {
	// Protobuf JSON omits scalar fields whose value is zero.
	if len(raw) == 0 {
		return 0, nil
	}
	var text string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, err
		}
	} else {
		text = string(raw)
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid traffic counter")
	}
	return value, nil
}
func collectTraffic(ctx context.Context, s *store.Store, binary, address string) (string, error) {
	users, err := s.Users(ctx)
	if err != nil {
		return "", err
	}
	deltas := map[string]*store.TrafficDelta{}
	observed := map[string]bool{}
	for _, user := range users {
		deltas[user.ID] = &store.TrafficDelta{UserID: user.ID}
	}
	output, err := xrayJSON(ctx, binary, address, "statsquery", "-pattern", "user>>>", "-reset")
	if err != nil {
		return "", err
	}
	var response struct {
		Stat []struct {
			Name  string
			Value json.RawMessage
		}
	}
	if err = json.Unmarshal(output, &response); err != nil {
		return "", err
	}
	for _, stat := range response.Stat {
		parts := strings.Split(stat.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			continue
		}
		id, credential, ok := emailUser(parts[1])
		if !ok || deltas[id] == nil {
			continue
		}
		value, err := counterValue(stat.Value)
		if err != nil {
			return "", err
		}
		observed[id] = true
		delta := deltas[id]
		switch credential + "." + parts[3] {
		case "main.uplink":
			delta.MainUp += value
		case "main.downlink":
			delta.MainDown += value
		case "limited.uplink":
			delta.LimitedUp += value
		case "limited.downlink":
			delta.LimitedDown += value
		}
	}
	// Persist reset counters before making another network request.
	list := make([]store.TrafficDelta, 0, len(deltas))
	for id, delta := range deltas {
		if observed[id] {
			list = append(list, *delta)
		}
	}
	if err = s.AddTraffic(ctx, time.Now(), list, nil); err != nil {
		return "", err
	}
	output, err = xrayJSON(ctx, binary, address, "statsonlineiplist", "-all")
	if err != nil {
		return err.Error(), nil
	}
	var online struct {
		Users []struct {
			Email string
			IPs   []struct {
				IP            string
				LastSeen      json.RawMessage `json:"lastSeen"`
				LastSeenSnake json.RawMessage `json:"last_seen"`
			}
		}
	}
	if err = json.Unmarshal(output, &online); err != nil {
		return err.Error(), nil
	}
	ips := []store.IPObservation{}
	for _, user := range online.Users {
		id, credential, ok := emailUser(user.Email)
		if !ok || deltas[id] == nil {
			continue
		}
		for _, ip := range user.IPs {
			raw := ip.LastSeen
			if len(raw) == 0 {
				raw = ip.LastSeenSnake
			}
			seen, err := counterValue(raw)
			if err == nil {
				ips = append(ips, store.IPObservation{UserID: id, Credential: credential, IP: ip.IP, LastSeenAt: seen})
			}
		}
	}
	return "", s.AddTraffic(ctx, time.Now(), nil, ips)
}
func (collector *Collector) Run(ctx context.Context, s *store.Store, binary, address string) {
	if address == "" {
		collector.mu.Lock()
		collector.Error = "API статистики Xray не настроен"
		collector.mu.Unlock()
		return
	}
	if binary == "" {
		binary = "xray"
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		onlineError, err := collectTraffic(ctx, s, binary, address)
		collector.mu.Lock()
		collector.OnlineError = onlineError
		if err != nil {
			collector.Error = err.Error()
		} else {
			collector.Error = ""
			collector.LastPoll = time.Now().Unix()
		}
		collector.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
