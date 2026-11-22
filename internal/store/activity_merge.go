package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func decodeActivity(raw []byte) (map[string]any, error) {
	var data map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("activity counters cannot be deleted")
	}
	return data, nil
}
func activityNumber(data map[string]any, key string) (int64, error) {
	value, ok := data[key].(json.Number)
	if !ok {
		return 0, fmt.Errorf("invalid activity counter %s", key)
	}
	number, err := value.Int64()
	if err != nil || number < 0 {
		return 0, fmt.Errorf("negative or invalid activity counter %s", key)
	}
	return number, nil
}
func mergeActivity(ctx context.Context, tx *sql.Tx, change Change) (Change, error) {
	incoming, err := decodeActivity(change.Data)
	if err != nil {
		return change, err
	}
	node, _ := incoming["node_id"].(string)
	user, _ := incoming["user_id"].(string)
	if len(node) != 36 || user == "" || incoming["id"] != change.Key {
		return change, fmt.Errorf("invalid activity identity")
	}
	keys := []string{"last_seen_at"}
	switch change.Table {
	case "xray_usage":
		period, _ := incoming["period"].(string)
		if len(period) != 7 || activityID(node, user, period) != change.Key {
			return change, fmt.Errorf("invalid usage period")
		}
		keys = append(keys, "main_up", "main_down", "limited_up", "limited_down", "sampled_at")
	case "xray_ips":
		ip, _ := incoming["ip"].(string)
		credential, _ := incoming["credential"].(string)
		if activityID(node, user, credential, ip) != change.Key {
			return change, fmt.Errorf("invalid IP record")
		}
	case "subscription_devices":
		hwid, _ := incoming["hwid"].(string)
		ua, _ := incoming["user_agent"].(string)
		identifier := hwid
		if identifier == "" {
			identifier = "no-hwid:" + ua
		}
		if activityID(node, user, identifier) != change.Key {
			return change, fmt.Errorf("invalid subscription record")
		}
		keys = append(keys, "first_seen_at", "request_count")
	}
	for _, key := range keys {
		if _, err := activityNumber(incoming, key); err != nil {
			return change, err
		}
	}
	var oldRaw string
	var oldVersion int64
	var oldOrigin string
	err = tx.QueryRowContext(ctx, "SELECT body,version,origin FROM replication_changes WHERE kind=? AND owner='' AND record_key=?", change.Table, change.Key).Scan(&oldRaw, &oldVersion, &oldOrigin)
	if errors.Is(err, sql.ErrNoRows) {
		return change, nil
	}
	if err != nil {
		return change, err
	}
	old, err := decodeActivity([]byte(oldRaw))
	if err != nil {
		return change, err
	}
	incomingSeen, _ := activityNumber(incoming, "last_seen_at")
	oldSeen, _ := activityNumber(old, "last_seen_at")
	result := incoming
	if oldSeen > incomingSeen || (oldSeen == incomingSeen && oldVersion > change.Version) {
		result = old
	}
	for _, key := range keys {
		left, err := activityNumber(old, key)
		if err != nil {
			return change, err
		}
		right, _ := activityNumber(incoming, key)
		number := max(left, right)
		if key == "first_seen_at" {
			number = min(left, right)
		}
		result[key] = json.Number(fmt.Sprint(number))
	}
	change.Version = max(change.Version, oldVersion)
	change.Origin = max(change.Origin, oldOrigin)
	change.Data, err = json.Marshal(result)
	return change, err
}
