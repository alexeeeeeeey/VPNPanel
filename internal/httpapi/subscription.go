package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/httpx"
	"vpnpanel/internal/subscription"
)

func mountSubscriptions(r chi.Router, sub *subscription.Service) {
	r.Get("/subscription", func(w http.ResponseWriter, r *http.Request) { httpx.Write(w, 200, sub.Status()) })
	r.Put("/subscription", func(w http.ResponseWriter, r *http.Request) {
		var settings subscription.Settings
		if !httpx.Read(w, r, &settings) {
			return
		}
		if err := sub.Apply(r.Context(), settings, true); err != nil {
			httpx.Fail(w, 400, err.Error())
			return
		}
		httpx.Write(w, 200, map[string]bool{"ok": true})
	})
}
