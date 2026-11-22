package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
)

func mountReplication(r chi.Router, s *store.Store) {
	r.Get("/sync", func(w http.ResponseWriter, req *http.Request) {
		changes, err := s.Changes(req.Context())
		if err != nil {
			httpx.Fail(w, 500, "Не удалось прочитать журнал")
			return
		}
		httpx.Write(w, 200, changes)
	})
	r.Post("/sync", func(w http.ResponseWriter, req *http.Request) {
		var changes []store.Change
		if !httpx.Read(w, req, &changes) {
			return
		}
		if err := s.Merge(req.Context(), changes); err != nil {
			httpx.Fail(w, 409, "Не удалось применить изменения: "+err.Error())
			return
		}
		httpx.Write(w, 200, map[string]bool{"ok": true})
	})
}
