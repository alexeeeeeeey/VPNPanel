package access

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html/template"
	"net"
	"net/http"
	"os"
	"strings"

	"vpnpanel/internal/httpx"
)

func mtlsProxyKey() (string, error) {
	key := os.Getenv("VPNPANEL_MTLS_PROXY_KEY")
	if path := os.Getenv("VPNPANEL_MTLS_PROXY_KEY_FILE"); path != "" {
		if key != "" {
			return "", errors.New("set only one mTLS proxy key source")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", errors.New("cannot read mTLS proxy key file")
		}
		key = strings.TrimSpace(string(raw))
	}
	if key != "" {
		raw, err := hex.DecodeString(key)
		if err != nil || len(raw) != 32 {
			return "", errors.New("mTLS proxy key must contain 64 hexadecimal characters")
		}
	}
	return key, nil
}

type certificateAccess struct{ key, prefix string }

func loopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && net.ParseIP(host).IsLoopback()
}

func (a certificateAccess) trustedProxy(r *http.Request) bool {
	return a.key != "" && loopbackRequest(r) && len(r.Header.Values("X-Panel-Proxy-Key")) == 1 && secretEqual(r.Header.Get("X-Panel-Proxy-Key"), a.key)
}

func (a certificateAccess) logoutCookie(r *http.Request) *http.Cookie {
	hash := sha256.Sum256([]byte(r.Host + a.prefix))
	return &http.Cookie{Name: "vpnpanel_signed_out_" + hex.EncodeToString(hash[:8]), Path: a.prefix + "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}

func (a certificateAccess) logoutValue(r *http.Request) string {
	mac := hmac.New(sha256.New, []byte(a.key))
	_, _ = mac.Write([]byte("signed-out:" + r.Host + a.prefix))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a certificateAccess) signedOut(r *http.Request) bool {
	cookie, err := r.Cookie(a.logoutCookie(r).Name)
	return err == nil && secretEqual(cookie.Value, a.logoutValue(r))
}

func (a certificateAccess) authorize(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	trusted := a.trustedProxy(r)
	verified := trusted && r.Header.Get("X-Panel-Client-Verify") == "SUCCESS" && r.Header.Get("X-Forwarded-Proto") == "https"
	// A local SSH CLI proves possession of the same node-local proxy secret.
	// nginx always clears this header, so it cannot bypass certificate checks.
	cli := trusted && r.Header.Get("X-Panel-CLI") == "1" && strings.HasPrefix(r.URL.Path, "/api/")
	if !verified && !cli {
		http.NotFound(w, r)
		return false
	}
	if cli {
		return true
	}
	if r.URL.Path == "/login" || r.URL.Path == "/logout" {
		if r.Method == "GET" && r.URL.Path == "/login" {
			if !a.signedOut(r) {
				http.Redirect(w, r, a.prefix+"/", http.StatusSeeOther)
			} else {
				a.loginPage(w)
			}
			return false
		}
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return false
		}
		if r.Header.Get("Origin") != "https://"+r.Host || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			httpx.Fail(w, http.StatusForbidden, "Недопустимый источник запроса")
			return false
		}
		cookie := a.logoutCookie(r)
		cookie.MaxAge = -1
		target := a.prefix + "/"
		if r.URL.Path == "/logout" {
			cookie.Value, cookie.MaxAge = a.logoutValue(r), 12*60*60
			target = a.prefix + "/login"
		}
		http.SetCookie(w, cookie)
		http.Redirect(w, r, target, http.StatusSeeOther)
		return false
	}
	if a.signedOut(r) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httpx.Fail(w, http.StatusUnauthorized, "Вход завершён")
		} else if r.URL.Path == "/favicon.svg" {
			return true
		} else {
			http.Redirect(w, r, a.prefix+"/login", http.StatusSeeOther)
		}
		return false
	}
	return true
}

var certificateLoginPage = template.Must(template.New("certificate-login").Parse(`<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="{{.}}/favicon.svg" type="image/svg+xml"><title>Panel</title><style>:root{color-scheme:dark}*{box-sizing:border-box}body{margin:0;min-height:100dvh;display:grid;place-items:center;padding:16px;background:#161616;color:#f4f4f4;font:14px Inter,"Segoe UI",system-ui,sans-serif}main{width:min(100%,390px);padding:32px;border:1px solid #383838;border-radius:14px;background:#202020}.brand{font-size:12px;font-weight:750;letter-spacing:.1em}h1{margin:12px 0 28px;font-size:30px;letter-spacing:-.04em}button{width:100%;padding:12px;border:0;border-radius:8px;background:#fff;color:#151515;font:600 14px Inter,"Segoe UI",system-ui;cursor:pointer}:focus-visible{outline:2px solid #fff;outline-offset:3px}</style></head><body><main><p class="brand">VPN PANEL</p><h1>Вход завершён</h1><form action="{{.}}/login" method="post"><button type="submit">Войти по сертификату</button></form></main></body></html>`))

func (a certificateAccess) loginPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	_ = certificateLoginPage.Execute(w, a.prefix)
}
