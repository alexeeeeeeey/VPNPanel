package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"sort"

	"vpnpanel/internal/config"
)

type Store struct{ DB *sql.DB }
type Identity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (s *Store) Identity() (Identity, error) {
	var result Identity
	err := s.DB.QueryRow("SELECT uuid,name,url FROM node_identity WHERE singleton=1").Scan(&result.ID, &result.Name, &result.URL)
	return result, err
}

type Template struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Body string `json:"body"`
}

func UUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000;
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS users (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, uuid TEXT NOT NULL UNIQUE,
 limited_uuid TEXT NOT NULL UNIQUE, enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS templates (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('client','server')), body TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS subscription_settings (
 id INTEGER PRIMARY KEY CHECK(id=1), body TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS panel_nodes (
 id TEXT PRIMARY KEY, url TEXT NOT NULL, api_key TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS panel_variables (
 id INTEGER PRIMARY KEY CHECK(id=1), body TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS node_config (
 id INTEGER PRIMARY KEY CHECK(id=1), template_id TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS template_order (
 kind TEXT PRIMARY KEY CHECK(kind IN ('client','server')), body TEXT NOT NULL
);
`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateUsers(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateNodes(db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS node_identity(singleton INTEGER PRIMARY KEY CHECK(singleton=1),uuid TEXT NOT NULL,name TEXT NOT NULL)"); err != nil {
		db.Close()
		return nil, err
	}
	hostname, _ := os.Hostname()
	if _, err := db.Exec("INSERT OR IGNORE INTO node_identity(singleton,uuid,name) VALUES(1,?,?)", UUID(), hostname); err != nil {
		db.Close()
		return nil, err
	}
	var hasURL bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pragma_table_info('node_identity') WHERE name='url')").Scan(&hasURL); err != nil {
		db.Close()
		return nil, err
	}
	if !hasURL {
		if _, err := db.Exec("ALTER TABLE node_identity ADD COLUMN url TEXT NOT NULL DEFAULT ''"); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS node_identity_uuid ON node_identity(uuid)"); err != nil {
		db.Close()
		return nil, err
	}
	result := &Store{DB: db}
	if err := result.initActivity(); err != nil {
		db.Close()
		return nil, err
	}
	if err := result.initReplication(); err != nil {
		db.Close()
		return nil, err
	}
	if err := result.migrateSharedTemplates(); err != nil {
		db.Close()
		return nil, err
	}
	return result, nil
}

func migrateNodes(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM pragma_table_info('panel_nodes') WHERE name='name')").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec("ALTER TABLE panel_nodes ADD COLUMN name TEXT NOT NULL DEFAULT ''; UPDATE panel_nodes SET name=id"); err != nil {
			return err
		}
		rows, err := tx.Query("SELECT id FROM panel_nodes")
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.Exec("UPDATE panel_nodes SET id=? WHERE id=?", UUID(), id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func migrateUsers(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM pragma_table_info('users') WHERE name='sub_id')").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec("ALTER TABLE users ADD COLUMN sub_id TEXT NOT NULL DEFAULT ''; UPDATE users SET sub_id=id"); err != nil {
			return err
		}
	}
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM pragma_table_info('users') WHERE name='expires_at')").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec("ALTER TABLE users ADD COLUMN expires_at INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	for _, column := range []string{"created_at", "limited_bytes"} {
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM pragma_table_info('users') WHERE name=?)", column).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec("ALTER TABLE users ADD COLUMN " + column + " INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS users_sub_id ON users(sub_id); PRAGMA user_version=7"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Users(ctx context.Context) ([]config.User, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,name,uuid,limited_uuid,enabled,sub_id,expires_at,created_at,limited_bytes FROM users ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []config.User{}
	for rows.Next() {
		var u config.User
		if err := rows.Scan(&u.ID, &u.Name, &u.UUID, &u.LimitedUUID, &u.Enabled, &u.SubID, &u.ExpiresAt, &u.CreatedAt, &u.LimitedBytes); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	summaries, err := s.ActivitySummaries(ctx)
	if err != nil {
		return nil, err
	}
	for index := range users {
		value := summaries[users[index].ID]
		users[index].TotalMain = value.TotalMain
		users[index].TotalLimited = value.TotalLimited
		users[index].QuotaLimited = value.QuotaLimited
		users[index].LastSeenAt = value.LastSeenAt
		users[index].SubscriptionLastSeenAt = value.SubscriptionLastSeenAt
		users[index].DeviceCount = value.DeviceCount
		users[index].UnknownClientCount = value.UnknownClientCount
		if value.TrafficKnown {
			quota := value.QuotaLimited
			users[index].LimitedUsedBytes = &quota
		}
	}
	return users, nil
}

func (s *Store) Templates(ctx context.Context) ([]Template, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,name,kind,body FROM templates ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Template{}
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Kind, &t.Body); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	var raw string
	order := map[string]int{}
	orderRows, err := s.DB.QueryContext(ctx, "SELECT body FROM template_order")
	if err != nil {
		return nil, err
	}
	defer orderRows.Close()
	for orderRows.Next() {
		if err := orderRows.Scan(&raw); err != nil {
			return nil, err
		}
		var ids []string
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			return nil, err
		}
		for index, id := range ids {
			order[id] = index + 1
		}
	}
	if err := orderRows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := order[items[i].ID], order[items[j].ID]
		if left == 0 {
			left = int(^uint(0) >> 1)
		}
		if right == 0 {
			right = int(^uint(0) >> 1)
		}
		return left < right
	})
	return items, nil
}
