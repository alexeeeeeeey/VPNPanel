package access

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCertificateAccessFailClosed(t *testing.T) {
	key := strings.Repeat("a", 64)
	t.Setenv("VPNPANEL_MTLS_PROXY_KEY", key)
	t.Setenv("VPNPANEL_MTLS_PROXY_KEY_FILE", "")
	t.Setenv("VPNPANEL_NODE_KEY", strings.Repeat("k", 32))
	// Old password settings cannot restore password authentication.
	t.Setenv("VPNPANEL_USER", "admin")
	t.Setenv("VPNPANEL_PASSWORD", "password")
	panel, err := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }), "/panel")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, address, path, proxy, verify, proto, cli, bearer string
		want                                                   int
	}{
		{"no certificate", "127.0.0.1:1", "/panel/", "", "", "", "", "", 404},
		{"no redirect leak", "127.0.0.1:1", "/panel", "", "", "", "", "", 404},
		{"no login form", "127.0.0.1:1", "/panel/login", "", "", "", "", "", 404},
		{"forged success", "127.0.0.1:1", "/panel/api/state", "wrong", "SUCCESS", "https", "", "", 404},
		{"missing certificate", "127.0.0.1:1", "/panel/", key, "NONE", "https", "", "", 404},
		{"wrong transport", "127.0.0.1:1", "/panel/", key, "SUCCESS", "http", "", "", 404},
		{"untrusted caller", "203.0.113.1:1", "/panel/", key, "SUCCESS", "https", "", "", 404},
		{"verified", "127.0.0.1:1", "/panel/", key, "SUCCESS", "https", "", "", 200},
		{"verified IPv6", "[::1]:1", "/panel/api/state", key, "SUCCESS", "https", "", "", 200},
		{"node key", "127.0.0.1:1", "/panel/node-api/sync", "", "", "", "", strings.Repeat("k", 32), 200},
		{"node key cannot open admin", "127.0.0.1:1", "/panel/api/state", "", "", "", "", strings.Repeat("k", 32), 404},
		{"local SSH CLI", "127.0.0.1:1", "/panel/api/state", key, "", "", "1", "", 200},
		{"CLI cannot open HTML", "127.0.0.1:1", "/panel/", key, "", "", "1", "", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://panel.example"+tc.path, nil)
			r.RemoteAddr = tc.address
			r.Header.Set("X-Panel-Proxy-Key", tc.proxy)
			r.Header.Set("X-Panel-Client-Verify", tc.verify)
			r.Header.Set("X-Forwarded-Proto", tc.proto)
			r.Header.Set("X-Panel-CLI", tc.cli)
			r.SetBasicAuth("admin", "password")
			if tc.bearer != "" {
				r.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			w := httptest.NewRecorder()
			panel.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
		})
	}
	t.Setenv("VPNPANEL_MTLS_PROXY_KEY", "")
	closed, err := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("unconfigured auth opened panel") }), "/panel")
	if err != nil {
		t.Fatal(err)
	}
	closed.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/panel/", nil))
}

func TestCertificateLogoutRequiresCertificateToReturn(t *testing.T) {
	a := certificateAccess{key: strings.Repeat("a", 64), prefix: "/panel"}
	call := func(method, path, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://panel.example"+path, nil)
		r.RemoteAddr = "127.0.0.1:1"
		r.Header.Set("X-Panel-Proxy-Key", a.key)
		r.Header.Set("X-Panel-Client-Verify", "SUCCESS")
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		if a.authorize(w, r) {
			w.WriteHeader(200)
		}
		return w
	}
	if call("POST", "/logout", "https://evil.example", nil).Code != 403 {
		t.Fatal("cross-site logout accepted")
	}
	w := call("POST", "/logout", "https://panel.example", nil)
	if w.Code != 303 || w.Header().Get("Location") != "/panel/login" {
		t.Fatal("logout failed")
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/panel/" {
		t.Fatal("invalid logout cookie")
	}
	if call("GET", "/api/state", "", cookie).Code != 401 {
		t.Fatal("signed-out API opened")
	}
	w = call("GET", "/login", "", cookie)
	if w.Code != 200 || strings.Contains(w.Body.String(), "password") || !strings.Contains(w.Body.String(), "Войти по сертификату") {
		t.Fatal("wrong reentry page")
	}
	if call("POST", "/login", "https://panel.example", cookie).Code != 303 {
		t.Fatal("certificate reentry failed")
	}
	if call("GET", "/logout", "", nil).Code != 405 {
		t.Fatal("GET logout accepted")
	}
}

func TestMTLSProxyKeyFile(t *testing.T) {
	t.Setenv("VPNPANEL_MTLS_PROXY_KEY", "")
	path := filepath.Join(t.TempDir(), "proxy.key")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 64)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VPNPANEL_MTLS_PROXY_KEY_FILE", path)
	if key, err := mtlsProxyKey(); err != nil || len(key) != 64 {
		t.Fatal("key file rejected", err)
	}
	t.Setenv("VPNPANEL_MTLS_PROXY_KEY", strings.Repeat("b", 64))
	if _, err := mtlsProxyKey(); err == nil {
		t.Fatal("ambiguous key sources accepted")
	}
}
