package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/config"
	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
	"vpnpanel/internal/validation"
)

const maxLimitedBytes = 1<<53 - 1

func mountUsers(r chi.Router, s *store.Store) {
	r.Post("/users", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name         string `json:"name"`
			SubID        string `json:"sub_id"`
			ExpiresAt    int64  `json:"expires_at"`
			LimitedBytes int64  `json:"limited_bytes"`
		}
		if !httpx.Read(w, r, &body) {
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" || len(body.Name) > 120 {
			httpx.Fail(w, 400, "Имя должно содержать от 1 до 120 байт")
			return
		}
		if body.SubID == "" {
			body.SubID = store.UUID()
		}
		if body.ExpiresAt < 0 || body.ExpiresAt > 253402300799 {
			httpx.Fail(w, 400, "Некорректный срок действия")
			return
		}
		if body.LimitedBytes < 0 || body.LimitedBytes > maxLimitedBytes {
			httpx.Fail(w, 400, "Некорректный лимит трафика")
			return
		}
		if !validation.SubID.MatchString(body.SubID) {
			httpx.Fail(w, 400, "sub_id: от 1 до 128 букв латиницы, цифр, дефисов или подчёркиваний")
			return
		}
		u := config.User{ID: store.UUID(), Name: body.Name, UUID: store.UUID(), LimitedUUID: store.UUID(), Enabled: true, SubID: body.SubID, ExpiresAt: body.ExpiresAt, CreatedAt: time.Now().Unix(), LimitedBytes: body.LimitedBytes}
		result, err := s.DB.ExecContext(r.Context(), "INSERT INTO users(id,name,uuid,limited_uuid,enabled,sub_id,expires_at,created_at,limited_bytes) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(sub_id) DO NOTHING", u.ID, u.Name, u.UUID, u.LimitedUUID, u.Enabled, u.SubID, u.ExpiresAt, u.CreatedAt, u.LimitedBytes)
		if err != nil {
			httpx.Fail(w, 500, "Не удалось создать пользователя")
			return
		}
		if count, _ := result.RowsAffected(); count == 0 {
			httpx.Fail(w, 409, "Этот sub_id уже используется")
			return
		}
		httpx.Write(w, 201, u)
	})
	r.Delete("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		result, err := s.DB.ExecContext(r.Context(), "DELETE FROM users WHERE id=?", chi.URLParam(r, "id"))
		if err != nil {
			httpx.Fail(w, 500, "Не удалось удалить пользователя")
			return
		}
		count, err := result.RowsAffected()
		if err != nil {
			httpx.Fail(w, 500, "Не удалось проверить удаление пользователя")
			return
		}
		if count == 0 {
			httpx.Fail(w, 404, "Пользователь не найден")
			return
		}
		httpx.Write(w, 200, map[string]bool{"ok": true})
	})
	r.Patch("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name         *string `json:"name"`
			Enabled      *bool   `json:"enabled"`
			SubID        *string `json:"sub_id"`
			ExpiresAt    *int64  `json:"expires_at"`
			LimitedBytes *int64  `json:"limited_bytes"`
			ResetMain    bool    `json:"reset_main"`
			ResetLimited bool    `json:"reset_limited"`
		}
		if !httpx.Read(w, r, &body) {
			return
		}
		if body.Name != nil {
			*body.Name = strings.TrimSpace(*body.Name)
			if *body.Name == "" || len(*body.Name) > 120 {
				httpx.Fail(w, 400, "Имя должно содержать от 1 до 120 байт")
				return
			}
		}
		if (body.Name == nil && body.Enabled == nil && body.SubID == nil && body.ExpiresAt == nil && body.LimitedBytes == nil && !body.ResetMain && !body.ResetLimited) ||
			(body.SubID != nil && !validation.SubID.MatchString(*body.SubID)) ||
			(body.LimitedBytes != nil && (*body.LimitedBytes < 0 || *body.LimitedBytes > maxLimitedBytes)) ||
			(body.ExpiresAt != nil && (*body.ExpiresAt < 0 || *body.ExpiresAt > 253402300799)) {
			httpx.Fail(w, 400, "Некорректные настройки пользователя")
			return
		}
		id := chi.URLParam(r, "id")
		var mainUUID, limitedUUID *string
		if body.ResetMain {
			value := store.UUID()
			mainUUID = &value
		}
		if body.ResetLimited {
			value := store.UUID()
			limitedUUID = &value
		}
		result, err := s.DB.ExecContext(r.Context(), "UPDATE OR IGNORE users SET name=COALESCE(?,name),enabled=COALESCE(?,enabled),sub_id=COALESCE(?,sub_id),expires_at=COALESCE(?,expires_at),uuid=COALESCE(?,uuid),limited_uuid=COALESCE(?,limited_uuid),limited_bytes=COALESCE(?,limited_bytes) WHERE id=?", body.Name, body.Enabled, body.SubID, body.ExpiresAt, mainUUID, limitedUUID, body.LimitedBytes, id)
		if err != nil {
			httpx.Fail(w, 500, "Не удалось изменить пользователя")
			return
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			var exists bool
			if err := s.DB.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE id=?)", id).Scan(&exists); err == nil && exists {
				httpx.Fail(w, 409, "Этот sub_id уже используется")
				return
			}
			httpx.Fail(w, 404, "Пользователь не найден")
			return
		}
		httpx.Write(w, 200, map[string]bool{"ok": true})
	})

}
