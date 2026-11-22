package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
)

func mountActivity(r chi.Router, s *store.Store) {
	r.Get("/users/{id}/activity", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		var exists bool
		if err := s.DB.QueryRowContext(req.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE id=?)", id).Scan(&exists); err != nil {
			httpx.Fail(w, 500, "Не удалось загрузить пользователя")
			return
		}
		if !exists {
			httpx.Fail(w, 404, "Пользователь не найден")
			return
		}
		result, err := s.UserActivity(req.Context(), id)
		if err != nil {
			httpx.Fail(w, 500, "Не удалось прочитать статистику")
			return
		}
		result.Period = store.BillingPeriod(time.Now())
		httpx.Write(w, 200, result)
	})
}
