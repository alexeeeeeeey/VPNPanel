package httpapi

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func MountFrontend(r chi.Router, web fs.FS, prefix string) {
	spa := func(w http.ResponseWriter, req *http.Request) {
		page, err := fs.ReadFile(web, "index.html")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if req.Method != http.MethodHead {
			_, _ = io.WriteString(w, strings.Replace(string(page), `<base href="/">`, `<base href="`+prefix+`/">`, 1))
		}
	}
	r.Get("/", spa)
	r.Head("/", spa)
	for _, path := range []string{"/configs", "/servers", "/users", "/subscriptions"} {
		for _, page := range []string{path, path + "/"} {
			r.Get(page, spa)
			r.Head(page, spa)
		}
	}
	r.Handle("/*", http.FileServer(http.FS(web)))
}
