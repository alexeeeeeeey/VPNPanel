package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// Node-owned configurations are copied into the journal on every peer, while
// only the owner projects them into its active tables.
type replicatedTable struct {
	name    string
	key     string
	columns []string
	owned   bool
}

var replicatedTables = []replicatedTable{
	{"xray_usage", "id", []string{"id", "node_id", "user_id", "period", "main_up", "main_down", "limited_up", "limited_down", "last_seen_at", "sampled_at"}, false},
	{"xray_ips", "id", []string{"id", "node_id", "user_id", "credential", "ip", "last_seen_at"}, false},
	{"subscription_devices", "id", []string{"id", "node_id", "user_id", "hwid", "user_agent", "device_os", "os_version", "model", "app_version", "locale", "ip", "first_seen_at", "last_seen_at", "request_count"}, false},
	{"users", "id", []string{"id", "name", "uuid", "limited_uuid", "enabled", "sub_id", "expires_at", "created_at", "limited_bytes"}, false},
	{"templates", "id", []string{"id", "name", "kind", "body"}, false},
	{"template_order", "kind", []string{"kind", "body"}, false},
	{"node_config", "id", []string{"id", "template_id"}, true},
	{"panel_variables", "id", []string{"id", "body"}, true},
	{"panel_nodes", "id", []string{"id", "name", "url", "api_key"}, false},
	{"node_identity", "uuid", []string{"singleton", "uuid", "name", "url"}, true},
}

type Change struct {
	Table   string          `json:"table"`
	Owner   string          `json:"owner"`
	Key     string          `json:"key"`
	Version int64           `json:"version"`
	Origin  string          `json:"origin"`
	Data    json.RawMessage `json:"data"`
}

func (s *Store) initReplication() error {
	identity, err := s.Identity()
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS replication_guard(id INTEGER PRIMARY KEY, importing INTEGER NOT NULL);
INSERT OR IGNORE INTO replication_guard VALUES(1,0);
CREATE TABLE IF NOT EXISTS replication_contacts(id TEXT PRIMARY KEY,name TEXT NOT NULL,url TEXT NOT NULL,api_key TEXT NOT NULL);
CREATE TRIGGER IF NOT EXISTS replicate_removed_contact BEFORE DELETE ON panel_nodes
WHEN (SELECT importing FROM replication_guard WHERE id=1)=0 BEGIN
INSERT INTO replication_contacts(id,name,url,api_key) VALUES(OLD.id,OLD.name,OLD.url,OLD.api_key)
ON CONFLICT(id) DO UPDATE SET name=excluded.name,url=excluded.url,api_key=excluded.api_key;
END;
CREATE TABLE IF NOT EXISTS replication_changes(
 kind TEXT NOT NULL, owner TEXT NOT NULL, record_key TEXT NOT NULL,
 version INTEGER NOT NULL, origin TEXT NOT NULL, body TEXT,
 PRIMARY KEY(kind,owner,record_key));`); err != nil {
		return err
	}
	for _, table := range replicatedTables {
		if table.name == "templates" {
			for _, op := range []string{"insert", "update", "delete"} {
				if _, err = tx.Exec("DROP TRIGGER IF EXISTS replicate_templates_" + op); err != nil {
					return err
				}
			}
		}
		owner := ""
		if table.owned {
			owner = identity.ID
		}
		parts := []string{}
		for _, column := range table.columns {
			parts = append(parts, fmt.Sprintf("'%s',NEW.%s", column, column))
		}
		body := "json_object(" + strings.Join(parts, ",") + ")"
		for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
			row := "NEW"
			data := body
			if operation == "DELETE" {
				row = "OLD"
				data = "NULL"
			}
			stmt := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS replicate_%s_%s AFTER %s ON %s
WHEN (SELECT importing FROM replication_guard WHERE id=1)=0 BEGIN
INSERT INTO replication_changes(kind,owner,record_key,version,origin,body)
VALUES('%s','%s',CAST(%s.%s AS TEXT),
MAX(CAST((julianday('now')-2440587.5)*86400000 AS INTEGER),COALESCE((SELECT MAX(version)+1 FROM replication_changes),0)),
'%s',%s)
ON CONFLICT(kind,owner,record_key) DO UPDATE SET version=excluded.version,origin=excluded.origin,body=excluded.body;
END`, table.name, strings.ToLower(operation), operation, table.name, table.name, owner, row, table.key, identity.ID, data)
			if _, err = tx.Exec(stmt); err != nil {
				return err
			}
		}
		seed := strings.ReplaceAll(body, "NEW.", "")
		stmt := fmt.Sprintf("INSERT OR IGNORE INTO replication_changes SELECT '%s','%s',CAST(%s AS TEXT),0,'%s',%s FROM %s", table.name, owner, table.key, identity.ID, seed, table.name)
		if _, err = tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Changes(ctx context.Context) ([]Change, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT kind,owner,record_key,version,origin,body FROM replication_changes WHERE kind<>'variable_types' ORDER BY kind,owner,record_key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Change{}
	for rows.Next() {
		var change Change
		var data sql.NullString
		if err := rows.Scan(&change.Table, &change.Owner, &change.Key, &change.Version, &change.Origin, &data); err != nil {
			return nil, err
		}
		if data.Valid {
			change.Data = json.RawMessage(data.String)
		} else {
			change.Data = json.RawMessage("null")
		}
		result = append(result, change)
	}
	return result, rows.Err()
}

func (s *Store) Merge(ctx context.Context, changes []Change) error {
	identity, err := s.Identity()
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE replication_guard SET importing=1 WHERE id=1"); err != nil {
		return err
	}
	for _, change := range changes {
		// Type metadata from older peers no longer affects substitution.
		if change.Table == "variable_types" {
			continue
		}
		if change.Table == "templates" {
			change.Owner = ""
		}
		var table *replicatedTable
		for i := range replicatedTables {
			if replicatedTables[i].name == change.Table {
				table = &replicatedTables[i]
				break
			}
		}
		if table == nil || change.Key == "" || change.Version < 0 || len(change.Origin) != 36 || (table.owned && len(change.Owner) != 36) || (!table.owned && change.Owner != "") {
			return fmt.Errorf("invalid replication record")
		}
		monotonic := change.Table == "xray_usage" || change.Table == "xray_ips" || change.Table == "subscription_devices"
		if monotonic {
			change, err = mergeActivity(ctx, tx, change)
			if err != nil {
				return err
			}
		}
		condition := "excluded.version>version OR (excluded.version=version AND excluded.origin>origin)"
		if monotonic {
			condition += " OR excluded.body<>body"
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO replication_changes(kind,owner,record_key,version,origin,body) VALUES(?,?,?,?,?,?)
ON CONFLICT(kind,owner,record_key) DO UPDATE SET version=excluded.version,origin=excluded.origin,body=excluded.body
WHERE `+condition,
			change.Table, change.Owner, change.Key, change.Version, change.Origin, nullableBody(change.Data))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			continue
		}
		if change.Table == "node_identity" && change.Owner != identity.ID {
			if string(change.Data) != "null" && len(change.Data) > 0 {
				var incoming Identity
				var row struct {
					UUID string `json:"uuid"`
					Name string `json:"name"`
					URL  string `json:"url"`
				}
				if err := json.Unmarshal(change.Data, &row); err != nil {
					return err
				}
				incoming.ID, incoming.Name, incoming.URL = row.UUID, row.Name, row.URL
				if _, err = tx.ExecContext(ctx, "UPDATE panel_nodes SET name=?,url=CASE WHEN ?='' THEN url ELSE ? END WHERE id=?", incoming.Name, incoming.URL, incoming.URL, incoming.ID); err != nil {
					return err
				}
			}
			continue
		}
		if table.owned && change.Owner != identity.ID {
			continue
		}
		if change.Table == "panel_nodes" && change.Key == identity.ID {
			continue
		}
		if string(change.Data) == "null" || len(change.Data) == 0 {
			if change.Table == "node_identity" {
				return fmt.Errorf("cannot delete node identity")
			}
			if _, err = tx.ExecContext(ctx, "DELETE FROM "+table.name+" WHERE "+table.key+"=?", change.Key); err != nil {
				return err
			}
			continue
		}
		var data map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(change.Data)))
		decoder.UseNumber()
		if err = decoder.Decode(&data); err != nil {
			return err
		}
		values := []any{}
		placeholders, updates := []string{}, []string{}
		for _, column := range table.columns {
			value, ok := data[column]
			if !ok {
				return fmt.Errorf("missing replicated column")
			}
			if number, ok := value.(json.Number); ok {
				value = string(number)
			}
			values = append(values, value)
			placeholders = append(placeholders, "?")
			if column != table.key {
				updates = append(updates, column+"=excluded."+column)
			}
		}
		if fmt.Sprint(data[table.key]) != change.Key {
			return fmt.Errorf("replication key mismatch")
		}
		query := "INSERT INTO " + table.name + "(" + strings.Join(table.columns, ",") + ") VALUES(" + strings.Join(placeholders, ",") + ") ON CONFLICT(" + table.key + ") DO UPDATE SET " + strings.Join(updates, ",")
		if _, err = tx.ExecContext(ctx, query, values...); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE replication_guard SET importing=0 WHERE id=1"); err != nil {
		return err
	}
	return tx.Commit()
}

func nullableBody(body json.RawMessage) any {
	if len(body) == 0 || string(body) == "null" {
		return nil
	}
	return string(body)
}

func (s *Store) migrateSharedTemplates() error {
	changes, err := s.Changes(context.Background())
	if err != nil {
		return err
	}
	legacy := []Change{}
	for _, change := range changes {
		if change.Table == "templates" && change.Owner != "" {
			legacy = append(legacy, change)
		}
	}
	if len(legacy) == 0 {
		return nil
	}
	if err = s.Merge(context.Background(), legacy); err != nil {
		return err
	}
	_, err = s.DB.Exec("DELETE FROM replication_changes WHERE kind='templates' AND owner<>''")
	return err
}
