package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/assignment"
	"vpnpanel/internal/config"
	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
)

func mountTemplates(r chi.Router, s *store.Store) {
	r.Get("/state", func(w http.ResponseWriter, r *http.Request) {
		users, err := s.Users(r.Context())
		if err != nil {
			httpx.Fail(w, 500, "Не удалось прочитать пользователей")
			return
		}
		templates, err := s.Templates(r.Context())
		if err != nil {
			httpx.Fail(w, 500, "Не удалось прочитать шаблоны")
			return
		}
		httpx.Write(w, 200, map[string]any{"users": users, "templates": templates})
	})
	r.Post("/templates", func(w http.ResponseWriter, r *http.Request) {
		var t store.Template
		if !httpx.Read(w, r, &t) {
			return
		}
		t.Name = strings.TrimSpace(t.Name)
		if t.Name == "" || len(t.Name) > 120 || (t.Kind != "client" && t.Kind != "server") {
			httpx.Fail(w, 400, "Укажите имя и тип шаблона")
			return
		}
		requirements, err := config.Requirements(t.Body)
		if err != nil {
			httpx.Fail(w, 400, err.Error())
			return
		}
		// Validate markers even when there are no real users yet.
		dummy := config.User{UUID: "validation", LimitedUUID: "validation-limited", Enabled: true}
		variables := map[string]any{}
		for _, name := range requirements {
			variables[name] = "null"
		}
		if _, err := config.RenderWithVariables(t.Body, t.Kind, nil, &dummy, variables); err != nil {
			httpx.Fail(w, 400, err.Error())
			return
		}
		if t.ID == "" {
			t.ID = store.UUID()
		}
		_, err = s.DB.ExecContext(r.Context(), `INSERT INTO templates(id,name,kind,body) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,kind=excluded.kind,body=excluded.body`, t.ID, t.Name, t.Kind, t.Body)
		if err != nil {
			httpx.Fail(w, 500, "Не удалось сохранить шаблон")
			return
		}
		httpx.Write(w, 200, t)
	})
	r.Post("/render", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Kind   string `json:"kind"`
			Body   string `json:"body"`
			UserID string `json:"user_id"`
		}
		if !httpx.Read(w, r, &body) {
			return
		}
		users, err := s.Users(r.Context())
		if err != nil {
			httpx.Fail(w, 500, "Не удалось прочитать пользователей")
			return
		}
		var selected *config.User
		for i := range users {
			if users[i].ID == body.UserID {
				selected = &users[i]
				break
			}
		}
		_, variables, err := assignment.Variables(r.Context(), s)
		if err != nil {
			httpx.Fail(w, 500, "Не удалось загрузить переменные")
			return
		}
		out, err := config.RenderWithVariables(body.Body, body.Kind, users, selected, variables)
		if err != nil {
			httpx.Fail(w, 400, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(out)
	})
}
