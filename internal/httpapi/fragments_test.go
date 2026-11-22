package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"vpnpanel/internal/store"
)

func TestSaveAndRenderRawFragmentTemplate(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.DB.Close() })
	router := testRoutes(t.Context(), s)
	request := func(path string, body any, want int) string {
		t.Helper()
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", path, strings.NewReader(string(payload)))
		req.Header.Set("X-Panel-Request", "1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	raw := `{"inbounds":[],$STATS}`
	request("/api/templates", map[string]string{"name": "Stats", "kind": "server", "body": raw}, 200)
	if _, err := s.DB.Exec("INSERT INTO panel_variables VALUES(1,?)", `{"STATS":"\"stats\":{},\"api\":{\"listen\":\"127.0.0.1:10085\",\"services\":[\"StatsService\"]}"}`); err != nil {
		t.Fatal(err)
	}
	out := request("/api/render", map[string]string{"kind": "server", "body": raw}, 200)
	if !strings.Contains(out, "StatsService") {
		t.Fatal("fragment missing from HTTP render")
	}
	if _, err := s.DB.Exec("UPDATE panel_variables SET body=?", `{"STATS":"broken"}`); err != nil {
		t.Fatal(err)
	}
	request("/api/render", map[string]string{"kind": "server", "body": raw}, 400)
}
