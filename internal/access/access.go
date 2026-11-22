package access

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"strings"

	"vpnpanel/internal/cluster"
	"vpnpanel/internal/httpx"
)

func secretEqual(a, b string) bool {
	left, right := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func New(next http.Handler, prefix string) (http.Handler, error) {
	key := os.Getenv("VPNPANEL_NODE_KEY")
	proxyKey, err := mtlsProxyKey()
	if err != nil {
		return nil, err
	}
	if key != "" && (len(key) < 32 || len(key) > 256 || strings.ContainsAny(key, " \t\r\n")) {
		return nil, fmt.Errorf("VPNPANEL_NODE_KEY must contain 32–256 characters without whitespace")
	}
	certificates := certificateAccess{key: proxyKey, prefix: prefix}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		redirectBase := false
		if prefix != "" {
			if path == prefix && (r.Method == "GET" || r.Method == "HEAD") {
				path = prefix + "/"
				redirectBase = true
			}
			if !strings.HasPrefix(path, prefix+"/") {
				http.NotFound(w, r)
				return
			}
			path = strings.TrimPrefix(path, prefix)
		}
		r = r.Clone(r.Context())
		r.URL.Path, r.URL.RawPath = path, ""
		if endpoint, ok := strings.CutPrefix(path, "/node-api/"); ok {
			if key == "" || !cluster.Operation(r.Method, endpoint) {
				http.NotFound(w, r)
				return
			}
			if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); !ok || !secretEqual(token, key) {
				httpx.Fail(w, http.StatusUnauthorized, "Неверный ключ узла")
				return
			}
			r.URL.Path = "/api/" + endpoint
			r.Header.Del("Authorization")
			r.Header.Set("X-Panel-Request", "1")
		} else if !certificates.authorize(w, r) {
			return
		}
		if redirectBase {
			http.Redirect(w, r, prefix+"/", http.StatusPermanentRedirect)
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}
