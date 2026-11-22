package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/access"
	"vpnpanel/internal/store"
)

func TestNodeProxyRoutesOnlyPanelOperations(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Route", r.Method+" "+r.URL.Path)
	}))
	t.Cleanup(remote.Close)
	t.Setenv("VPNPANEL_NODES", `{"test":"`+remote.URL+`"}`)
	router := chi.NewRouter()
	s, err := store.Open(filepath.Join(t.TempDir(), "nodes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.DB.Close() })
	router.Route("/api", func(r chi.Router) { mountNodes(r, s.DB) })
	for _, check := range []struct {
		method, endpoint, remoteRoute string
		status                        int
	}{
		{"GET", "status", "GET /api/status", 200},
		{"POST", "templates", "POST /api/templates", 200},
		{"GET", "update-status", "GET /api/update-status", 200},
		{"POST", "update", "POST /api/update", 200},
		{"POST", "users", "", 404},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(check.method, "/api/nodes/test/"+check.endpoint, nil))
		if w.Code != check.status || w.Header().Get("X-Test-Route") != check.remoteRoute {
			t.Errorf("%s %s: status %d, forwarded %q", check.method, check.endpoint, w.Code, w.Header().Get("X-Test-Route"))
		}
	}
}

func TestNodeInventoryAndScopedAccess(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"node_id":"fa6bd25b-5ec7-4a04-afb3-e02cd943e980"}`))
	}))
	t.Cleanup(remote.Close)
	key := strings.Repeat("k", 32)
	t.Setenv("VPNPANEL_NODE_KEY", key)
	t.Setenv("VPNPANEL_MTLS_PROXY_KEY", strings.Repeat("a", 64))
	s, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.DB.Close() })
	panel, err := access.New(testRoutes(t.Context(), s), "/panel")
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, auth string, want int) string {
		t.Helper()
		body = strings.ReplaceAll(body, "https://node.example/panel", remote.URL+"/panel")
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("X-Panel-Request", "1")
		req.RemoteAddr = "127.0.0.1:12345"
		switch auth {
		case "admin":
			req.Header.Set("X-Panel-Proxy-Key", strings.Repeat("a", 64))
			req.Header.Set("X-Panel-CLI", "1")
		case "peer":
			req.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		panel.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: status %d, expected %d", method, path, w.Code, want)
		}
		return w.Body.String()
	}
	call("GET", "/panel/node-api/status", "", "", 401)
	call("GET", "/panel/node-api/status", "", "peer", 200)
	call("GET", "/panel/api/nodes", "", "peer", 404)
	created := call("POST", "/panel/api/nodes", `{"name":"peer-1","url":"https://node.example/panel","key":"`+key+`"}`, "admin", 200)
	var record struct{ ID string }
	if json.Unmarshal([]byte(created), &record) != nil || len(record.ID) != 36 {
		t.Fatal("expected UUID for node connection")
	}
	if data := call("GET", "/panel/api/nodes", "", "admin", 200); !strings.Contains(data, "peer-1") || strings.Contains(data, key) {
		t.Fatal("invalid node inventory response")
	}
	path := "/panel/api/nodes/" + record.ID
	call("PUT", path, `{"name":"Новое имя","url":"https://node.example/panel"}`, "admin", 200)
	if data := call("GET", "/panel/api/nodes", "", "admin", 200); !strings.Contains(data, record.ID) || !strings.Contains(data, "Новое имя") {
		t.Fatal("rename changed node identity")
	}
	call("DELETE", path, "", "admin", 200)
	call("DELETE", path, "", "admin", 404)
}
