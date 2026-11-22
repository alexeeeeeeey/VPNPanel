package store

import (
	"context"
	"sort"
)

type UserIP struct {
	NodeID     string `json:"node_id"`
	Credential string `json:"credential"`
	IP         string `json:"ip"`
	LastSeenAt int64  `json:"last_seen_at"`
}
type UserActivity struct {
	UserSummary
	Period      string               `json:"period"`
	IPs         []UserIP             `json:"ips"`
	Devices     []SubscriptionDevice `json:"devices"`
	WithoutHWID []SubscriptionDevice `json:"without_hwid"`
}

func (s *Store) UserActivity(ctx context.Context, userID string) (UserActivity, error) {
	summaries, err := s.ActivitySummaries(ctx)
	result := UserActivity{UserSummary: summaries[userID], IPs: []UserIP{}, Devices: []SubscriptionDevice{}, WithoutHWID: []SubscriptionDevice{}}
	if err != nil {
		return result, err
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT node_id,credential,ip,last_seen_at FROM xray_ips WHERE user_id=? ORDER BY last_seen_at DESC,ip", userID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var ip UserIP
		if err = rows.Scan(&ip.NodeID, &ip.Credential, &ip.IP, &ip.LastSeenAt); err != nil {
			rows.Close()
			return result, err
		}
		result.IPs = append(result.IPs, ip)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT hwid,user_agent,device_os,os_version,model,app_version,locale,ip,first_seen_at,last_seen_at,request_count
FROM subscription_devices WHERE user_id=? ORDER BY last_seen_at DESC,id`, userID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	devices := map[string]SubscriptionDevice{}
	for rows.Next() {
		var device SubscriptionDevice
		if err = rows.Scan(&device.HWID, &device.UserAgent, &device.OS, &device.OSVersion, &device.Model, &device.AppVersion, &device.Locale, &device.IP, &device.FirstSeenAt, &device.LastSeenAt, &device.RequestCount); err != nil {
			return result, err
		}
		id := "hwid:" + device.HWID
		if device.HWID == "" {
			id = "ua:" + device.UserAgent
		}
		if previous, ok := devices[id]; ok {
			latest := device
			if previous.LastSeenAt >= device.LastSeenAt {
				latest = previous
			}
			latest.FirstSeenAt = min(previous.FirstSeenAt, device.FirstSeenAt)
			latest.RequestCount = previous.RequestCount + device.RequestCount
			devices[id] = latest
		} else {
			devices[id] = device
		}
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	for _, device := range devices {
		if device.HWID != "" {
			result.Devices = append(result.Devices, device)
		} else {
			result.WithoutHWID = append(result.WithoutHWID, device)
		}
	}
	for _, list := range [][]SubscriptionDevice{result.Devices, result.WithoutHWID} {
		sort.Slice(list, func(i, j int) bool {
			if list[i].LastSeenAt != list[j].LastSeenAt {
				return list[i].LastSeenAt > list[j].LastSeenAt
			}
			return list[i].UserAgent < list[j].UserAgent
		})
	}
	return result, nil
}
