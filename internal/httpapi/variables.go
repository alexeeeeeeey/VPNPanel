package httpapi

import (
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/assignment"
	"vpnpanel/internal/config"
	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
)

var variableName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

func mountVariables(r chi.Router, s *store.Store) {
	r.Get("/variables", func(w http.ResponseWriter, req *http.Request) {
		raw, _, err := assignment.Variables(req.Context(), s)
		if err != nil {
			httpx.Fail(w, 500, "Не удалось загрузить переменные")
			return
		}
		httpx.Write(w, 200, map[string]string{"body": raw})
	})
	r.Put("/variables", func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			Body string `json:"body"`
		}
		if !httpx.Read(w, req, &input) {
			return
		}
		values, err := config.Decode(input.Body)
		if err != nil {
			httpx.Fail(w, 400, err.Error())
			return
		}
		for name := range values {
			if !variableName.MatchString(name) {
				httpx.Fail(w, 400, "Недопустимое имя переменной: "+name)
				return
			}
			if config.SystemVariable(name) {
				httpx.Fail(w, 400, "Системная переменная: "+name)
				return
			}
		}
		if _, err := s.DB.ExecContext(req.Context(), "INSERT INTO panel_variables(id,body) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", input.Body); err != nil {
			httpx.Fail(w, 500, "Не удалось сохранить переменные")
			return
		}
		httpx.Write(w, 200, input)
	})
}
