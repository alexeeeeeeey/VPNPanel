package store

import (
	"path/filepath"
	"testing"
)

func TestReplicationOfflineEditsDeletesAndNodeOwnership(t *testing.T) {
	open := func(name string) *Store {
		s, err := Open(filepath.Join(t.TempDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.DB.Close() })
		return s
	}
	a, b := open("a.db"), open("b.db")
	exec := func(s *Store, query string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	merge := func(to, from *Store) {
		t.Helper()
		changes, err := from.Changes(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err = to.Merge(t.Context(), changes); err != nil {
			t.Fatal(err)
		}
	}
	exec(a, "INSERT INTO users(id,name,uuid,limited_uuid,enabled,sub_id) VALUES('user','A','main','limited',1,'sub')")
	exec(a, "INSERT INTO templates VALUES('config','A config','client','{}')")
	exec(a, "INSERT INTO template_order VALUES('client','[\"config\"]')")
	exec(a, "INSERT INTO panel_variables VALUES(1,'{}')")
	exec(a, "INSERT INTO node_config VALUES(1,'config')")
	exec(a, "INSERT INTO subscription_settings VALUES(1,'{}')")
	merge(b, a)
	merge(b, a)
	users, err := b.Users(t.Context())
	if err != nil || len(users) != 1 {
		t.Fatal("user not replicated", err)
	}
	templates, err := b.Templates(t.Context())
	if err != nil || len(templates) != 1 {
		t.Fatal("shared config missing from library", err)
	}
	var ordering string
	if err = b.DB.QueryRow("SELECT body FROM template_order WHERE kind='client'").Scan(&ordering); err != nil || ordering != `["config"]` {
		t.Fatal("order not replicated", err)
	}
	var count int
	if err = b.DB.QueryRow("SELECT COUNT(*) FROM node_config").Scan(&count); err != nil || count != 0 {
		t.Fatal("foreign assignment became active", err)
	}
	exec(b, "UPDATE templates SET name='Shared edit' WHERE id='config'")
	merge(a, b)
	templates, err = a.Templates(t.Context())
	if err != nil || templates[0].Name != "Shared edit" {
		t.Fatal("shared template change not returned", err)
	}
	if err = b.DB.QueryRow("SELECT COUNT(*) FROM subscription_settings").Scan(&count); err != nil || count != 0 {
		t.Fatal("local settings replicated", err)
	}
	exec(b, "UPDATE users SET name='B' WHERE id='user'")
	merge(a, b)
	exec(a, "UPDATE users SET name='latest' WHERE id='user'")
	merge(b, a)
	users, err = b.Users(t.Context())
	if err != nil || users[0].Name != "latest" {
		t.Fatal("latest edit did not win", err)
	}
	exec(b, "DELETE FROM users WHERE id='user'")
	merge(a, b)
	merge(b, a)
	users, err = a.Users(t.Context())
	if err != nil || len(users) != 0 {
		t.Fatal("deleted user resurrected", err)
	}
	changes, err := b.Changes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, change := range changes {
		if change.Table == "templates" && change.Key == "config" {
			found = true
		}
	}
	if !found {
		t.Fatal("foreign config not retained in replica journal")
	}
}
