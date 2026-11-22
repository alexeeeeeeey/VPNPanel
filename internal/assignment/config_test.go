package assignment

import (
	"path/filepath"
	"testing"

	"vpnpanel/internal/store"
)

func TestAssignmentRequiresAllValuesAndTracksSharedEdits(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO templates VALUES('base','Base','server',?)", `{"inbounds":[{"listen":"$LISTEN","port":$PORT,"clients":$CLIENTS_MAIN}]}`)
	exec("INSERT INTO node_config VALUES(1,'base')")
	state, out := ActiveConfig(t.Context(), s)
	if state.Ready || len(state.Missing) != 2 || out != nil {
		t.Fatal("unconfigured server became ready")
	}
	exec("INSERT INTO panel_variables VALUES(1,?)", `{"LISTEN":"127.0.0.1","PORT":443}`)
	state, out = ActiveConfig(t.Context(), s)
	if !state.Ready || len(out) == 0 {
		t.Fatal("configured server not ready", state)
	}
	exec("UPDATE templates SET body=? WHERE id='base'", `{"gateway":"$GATEWAY"}`)
	state, out = ActiveConfig(t.Context(), s)
	if state.Ready || len(state.Missing) != 1 || state.Missing[0] != "GATEWAY" || out != nil {
		t.Fatal("shared edit bypassed variable guard")
	}
}
