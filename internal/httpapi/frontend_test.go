package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
)

func TestFrontendFromFilesystem(t *testing.T) {
	web := fstest.MapFS{
		"index.html":    {Data: []byte(`<html><head><base href="/"></head><body>panel</body></html>`)},
		"assets/app.js": {Data: []byte(`console.log("panel")`)},
	}
	for _, prefix := range []string{"", "/panel"} {
		t.Run(prefix, func(t *testing.T) {
			router := chi.NewRouter()
			MountFrontend(router, web, prefix)
			var handler http.Handler = router
			if prefix != "" {
				handler = http.StripPrefix(prefix, router)
			}
			for _, route := range []string{"/", "/configs", "/servers/", "/users", "/subscriptions/"} {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, prefix+route, nil))
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `<base href="`+prefix+`/">`) {
					t.Fatalf("%s: status %d, body %s", route, w.Code, w.Body.String())
				}
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodHead, prefix+"/configs", nil))
			if w.Code != http.StatusOK || w.Body.Len() != 0 {
				t.Fatalf("HEAD: status %d, body %s", w.Code, w.Body.String())
			}
			w = httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, prefix+"/assets/app.js", nil))
			if w.Code != http.StatusOK || w.Body.String() != `console.log("panel")` {
				t.Fatalf("asset: status %d, body %s", w.Code, w.Body.String())
			}
		})
	}
}
