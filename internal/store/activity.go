package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

func BillingPeriod(at time.Time) string {
	zone, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		zone = time.FixedZone("MSK", 3*60*60)
	}
	return at.In(zone).Format("2006-01")
}

func activityID(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(hash, "%d:%s", len(part), part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Store) initActivity() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS xray_usage(
id TEXT PRIMARY KEY,node_id TEXT NOT NULL,user_id TEXT NOT NULL,period TEXT NOT NULL,
main_up INTEGER NOT NULL,main_down INTEGER NOT NULL,limited_up INTEGER NOT NULL,limited_down INTEGER NOT NULL,
last_seen_at INTEGER NOT NULL,sampled_at INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS xray_usage_user ON xray_usage(user_id,period);
CREATE TABLE IF NOT EXISTS xray_ips(
id TEXT PRIMARY KEY,node_id TEXT NOT NULL,user_id TEXT NOT NULL,credential TEXT NOT NULL,
ip TEXT NOT NULL,last_seen_at INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS xray_ips_user ON xray_ips(user_id);
CREATE TABLE IF NOT EXISTS subscription_devices(
id TEXT PRIMARY KEY,node_id TEXT NOT NULL,user_id TEXT NOT NULL,hwid TEXT NOT NULL,
user_agent TEXT NOT NULL,device_os TEXT NOT NULL,os_version TEXT NOT NULL,model TEXT NOT NULL,
app_version TEXT NOT NULL,locale TEXT NOT NULL,ip TEXT NOT NULL,
first_seen_at INTEGER NOT NULL,last_seen_at INTEGER NOT NULL,request_count INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS subscription_devices_user ON subscription_devices(user_id);`)
	return err
}

type TrafficDelta struct {
	UserID                                   string
	MainUp, MainDown, LimitedUp, LimitedDown int64
}
type IPObservation struct {
	UserID     string
	Credential string
	IP         string
	LastSeenAt int64
}

func (s *Store) AddTraffic(ctx context.Context, at time.Time, deltas []TrafficDelta, ips []IPObservation) error {
	identity, err := s.Identity()
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	period := BillingPeriod(at)
	for _, delta := range deltas {
		if delta.MainUp < 0 || delta.MainDown < 0 || delta.LimitedUp < 0 || delta.LimitedDown < 0 {
			return fmt.Errorf("negative traffic delta")
		}
		lastSeen := int64(0)
		if delta.MainUp+delta.MainDown+delta.LimitedUp+delta.LimitedDown > 0 {
			lastSeen = at.Unix()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO xray_usage VALUES(?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET main_up=main_up+excluded.main_up,main_down=main_down+excluded.main_down,
limited_up=limited_up+excluded.limited_up,limited_down=limited_down+excluded.limited_down,
last_seen_at=MAX(last_seen_at,excluded.last_seen_at),sampled_at=MAX(sampled_at,excluded.sampled_at)`,
			activityID(identity.ID, delta.UserID, period), identity.ID, delta.UserID, period,
			delta.MainUp, delta.MainDown, delta.LimitedUp, delta.LimitedDown, lastSeen, at.Unix())
		if err != nil {
			return err
		}
	}
	for _, ip := range ips {
		_, err = tx.ExecContext(ctx, `INSERT INTO xray_ips VALUES(?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET last_seen_at=MAX(last_seen_at,excluded.last_seen_at)`,
			activityID(identity.ID, ip.UserID, ip.Credential, ip.IP), identity.ID, ip.UserID, ip.Credential, ip.IP, ip.LastSeenAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

type SubscriptionDevice struct {
	HWID         string `json:"hwid"`
	UserAgent    string `json:"user_agent"`
	OS           string `json:"os"`
	OSVersion    string `json:"os_version"`
	Model        string `json:"model"`
	AppVersion   string `json:"app_version"`
	Locale       string `json:"locale"`
	IP           string `json:"ip"`
	FirstSeenAt  int64  `json:"first_seen_at"`
	LastSeenAt   int64  `json:"last_seen_at"`
	RequestCount int64  `json:"request_count"`
}

func (s *Store) RecordSubscription(ctx context.Context, userID string, at time.Time, device SubscriptionDevice) error {
	identity, err := s.Identity()
	if err != nil {
		return err
	}
	identifier := device.HWID
	if identifier == "" {
		identifier = "no-hwid:" + device.UserAgent
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO subscription_devices VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET user_agent=excluded.user_agent,device_os=excluded.device_os,os_version=excluded.os_version,
model=excluded.model,app_version=excluded.app_version,locale=excluded.locale,ip=excluded.ip,
last_seen_at=MAX(last_seen_at,excluded.last_seen_at),request_count=request_count+1`,
		activityID(identity.ID, userID, identifier), identity.ID, userID, device.HWID, device.UserAgent, device.OS,
		device.OSVersion, device.Model, device.AppVersion, device.Locale, device.IP, at.Unix(), at.Unix(), 1)
	return err
}

type UserSummary struct {
	TotalMain              int64 `json:"total_main"`
	TotalLimited           int64 `json:"total_limited"`
	QuotaLimited           int64 `json:"quota_limited"`
	LastSeenAt             int64 `json:"last_seen_at"`
	SubscriptionLastSeenAt int64 `json:"subscription_last_seen_at"`
	DeviceCount            int   `json:"device_count"`
	UnknownClientCount     int   `json:"unknown_client_count"`
	TrafficKnown           bool  `json:"traffic_known"`
}

func (s *Store) ActivitySummaries(ctx context.Context) (map[string]UserSummary, error) {
	result := map[string]UserSummary{}
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id,SUM(main_up+main_down),SUM(limited_up+limited_down),
SUM(CASE WHEN period=? THEN limited_up+limited_down ELSE 0 END),MAX(last_seen_at) FROM xray_usage GROUP BY user_id`, BillingPeriod(time.Now()))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var value UserSummary
		if err = rows.Scan(&id, &value.TotalMain, &value.TotalLimited, &value.QuotaLimited, &value.LastSeenAt); err != nil {
			rows.Close()
			return nil, err
		}
		value.TrafficKnown = true
		result[id] = value
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = s.DB.QueryContext(ctx, "SELECT user_id,MAX(last_seen_at) FROM xray_ips GROUP BY user_id")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var seen int64
		if err = rows.Scan(&id, &seen); err != nil {
			rows.Close()
			return nil, err
		}
		value := result[id]
		value.LastSeenAt = max(value.LastSeenAt, seen)
		result[id] = value
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT user_id,MAX(last_seen_at),
COUNT(DISTINCT CASE WHEN hwid<>'' THEN hwid END),
COUNT(DISTINCT CASE WHEN hwid='' THEN user_agent END) FROM subscription_devices GROUP BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var seen int64
		var known, unknown int
		if err = rows.Scan(&id, &seen, &known, &unknown); err != nil {
			return nil, err
		}
		value := result[id]
		value.SubscriptionLastSeenAt = seen
		value.DeviceCount = known
		value.UnknownClientCount = unknown
		result[id] = value
	}
	return result, rows.Err()
}
