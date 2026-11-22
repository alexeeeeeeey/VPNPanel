package subscription

import (
	"net"
	"net/http"
	"strings"
	"time"

	"vpnpanel/internal/store"
)

func shortHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			runes := []rune(strings.ToValidUTF8(value, ""))
			if len(runes) > 512 {
				runes = runes[:512]
			}
			return string(runes)
		}
	}
	return ""
}
func subscriptionIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if net.ParseIP(host).IsLoopback() {
		if ip := net.ParseIP(r.Header.Get("X-Real-IP")); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}
func recordSubscription(s *store.Store, r *http.Request, userID string) error {
	return s.RecordSubscription(r.Context(), userID, time.Now(), store.SubscriptionDevice{
		HWID:      shortHeader(r, "X-HWID", "X-Device-ID", "X-Client-HWID"),
		UserAgent: shortHeader(r, "User-Agent"),
		OS:        shortHeader(r, "X-Device-OS"), OSVersion: shortHeader(r, "X-Ver-OS"),
		Model:      shortHeader(r, "X-Device-Model", "X-Device-Name"),
		AppVersion: shortHeader(r, "X-App-Version"), Locale: shortHeader(r, "X-Device-Locale"),
		IP: subscriptionIP(r),
	})
}
