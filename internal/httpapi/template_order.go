package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
)

func mountTemplateOrder(r chi.Router, s *store.Store) {
	r.Put("/template-order", func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			Kind string   `json:"kind"`
			IDs  []string `json:"ids"`
		}
		if !httpx.Read(w, req, &input) {
			return
		}
		if input.Kind != "client" && input.Kind != "server" {
			httpx.Fail(w, 400, "Некорректный раздел библиотеки")
			return
		}
		tx, err := s.DB.BeginTx(req.Context(), nil)
		if err != nil {
			httpx.Fail(w, 500, "Не удалось сохранить порядок")
			return
		}
		defer tx.Rollback()
		rows, err := tx.QueryContext(req.Context(), "SELECT id FROM templates WHERE kind=?", input.Kind)
		if err != nil {
			httpx.Fail(w, 500, "Не удалось прочитать библиотеку")
			return
		}
		expected := map[string]bool{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				httpx.Fail(w, 500, "Не удалось прочитать библиотеку")
				return
			}
			expected[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			httpx.Fail(w, 500, "Не удалось прочитать библиотеку")
			return
		}
		for _, id := range input.IDs {
			if !expected[id] {
				httpx.Fail(w, 409, "Список конфигов изменился. Обновите библиотеку")
				return
			}
			delete(expected, id)
		}
		if len(expected) != 0 {
			httpx.Fail(w, 409, "Список конфигов изменился. Обновите библиотеку")
			return
		}
		raw, _ := json.Marshal(input.IDs)
		if _, err = tx.ExecContext(req.Context(), "INSERT INTO template_order(kind,body) VALUES(?,?) ON CONFLICT(kind) DO UPDATE SET body=excluded.body", input.Kind, string(raw)); err != nil {
			httpx.Fail(w, 500, "Не удалось сохранить порядок")
			return
		}
		if err = tx.Commit(); err != nil {
			httpx.Fail(w, 500, "Не удалось сохранить порядок")
			return
		}
		httpx.Write(w, 200, map[string]bool{"ok": true})
	})
}
