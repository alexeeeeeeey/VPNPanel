package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/assignment"
	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
)

func mountAssignment(r chi.Router, s *store.Store) {
	r.Get("/assignment", func(w http.ResponseWriter, req *http.Request) {
		state, _ := assignment.ActiveConfig(req.Context(), s)
		httpx.Write(w, 200, state)
	})
	r.Put("/assignment", func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			TemplateID string `json:"template_id"`
		}
		if !httpx.Read(w, req, &input) {
			return
		}
		if input.TemplateID != "" {
			var exists bool
			if err := s.DB.QueryRowContext(req.Context(), "SELECT EXISTS(SELECT 1 FROM templates WHERE id=? AND kind='server')", input.TemplateID).Scan(&exists); err != nil || !exists {
				httpx.Fail(w, 400, "Выберите серверный конфиг из библиотеки")
				return
			}
		}
		if _, err := s.DB.ExecContext(req.Context(), "INSERT INTO node_config VALUES(1,?) ON CONFLICT(id) DO UPDATE SET template_id=excluded.template_id", input.TemplateID); err != nil {
			httpx.Fail(w, 500, "Не удалось сохранить назначение")
			return
		}
		state, _ := assignment.ActiveConfig(req.Context(), s)
		httpx.Write(w, 200, state)
	})
}
