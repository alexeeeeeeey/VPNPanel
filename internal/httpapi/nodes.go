package httpapi

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/cluster"
	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
	"vpnpanel/internal/validation"
)

func mountNodes(r chi.Router, db *sql.DB) {
	legacy := map[string]cluster.Peer{}
	var addresses map[string]string
	var inventoryError string
	raw := os.Getenv("VPNPANEL_NODES")
	if raw != "" && json.Unmarshal([]byte(raw), &addresses) != nil {
		inventoryError = "Неверный формат VPNPANEL_NODES"
	}
	for id, address := range addresses {
		p := cluster.Peer{ID: id, Name: id, URL: address}
		if _, err := p.Validate(); err == nil {
			legacy[id] = p
		} else {
			inventoryError = "Недопустимый адрес в VPNPANEL_NODES"
		}
	}
	r.Get("/nodes", func(w http.ResponseWriter, req *http.Request) {
		all := map[string]cluster.Peer{}
		for id, p := range legacy {
			all[id] = p
		}
		rows, err := db.QueryContext(req.Context(), "SELECT id,name,url FROM panel_nodes")
		if err != nil {
			httpx.Fail(w, 500, "Не удалось загрузить серверы")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var p cluster.Peer
			if err := rows.Scan(&p.ID, &p.Name, &p.URL); err != nil {
				httpx.Fail(w, 500, "Не удалось загрузить серверы")
				return
			}
			all[p.ID] = p
		}
		if rows.Err() != nil {
			httpx.Fail(w, 500, "Не удалось загрузить серверы")
			return
		}
		var self store.Identity
		if err := db.QueryRowContext(req.Context(), "SELECT uuid,name,url FROM node_identity WHERE singleton=1").Scan(&self.ID, &self.Name, &self.URL); err != nil {
			httpx.Fail(w, 500, "Не удалось загрузить настройки узла")
			return
		}
		nodes := []string{"local"}
		for id := range all {
			nodes = append(nodes, id)
		}
		slices.Sort(nodes[1:])
		items := make([]map[string]any, 0, len(all))
		for _, id := range nodes[1:] {
			_, inherited := legacy[id]
			items = append(items, map[string]any{"id": id, "name": all[id].Name, "url": all[id].URL, "managed": !inherited})
		}
		httpx.Write(w, 200, map[string]any{"nodes": nodes, "items": items, "self": self, "error": inventoryError})
	})
	save := func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			cluster.Peer
			Code string `json:"code"`
		}
		if !httpx.Read(w, req, &input) {
			return
		}
		p := input.Peer
		if chi.URLParam(req, "id") == "local" {
			p.Name = strings.TrimSpace(p.Name)
			if p.Name == "" || len(p.Name) > 120 || input.Code != "" || p.Key != "" {
				httpx.Fail(w, 400, "Укажите имя и публичный URL своего узла")
				return
			}
			if p.URL != "" {
				u, err := url.Parse(p.URL)
				if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" ||
					(u.Path != "" && !validation.Path.MatchString(u.Path)) ||
					(u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback())) {
					httpx.Fail(w, 400, "Некорректный публичный URL")
					return
				}
			}
			if _, err := db.ExecContext(req.Context(), "UPDATE node_identity SET name=?,url=? WHERE singleton=1", p.Name, p.URL); err != nil {
				httpx.Fail(w, 500, "Не удалось сохранить настройки узла")
				return
			}
			httpx.Write(w, 200, map[string]bool{"ok": true})
			return
		}
		if input.Code != "" {
			var decoded []byte
			var err error
			for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding} {
				decoded, err = encoding.DecodeString(strings.TrimSpace(input.Code))
				if err == nil {
					break
				}
			}
			if err != nil || json.Unmarshal(decoded, &p) != nil {
				httpx.Fail(w, 400, "Некорректная строка подключения")
				return
			}
		}
		id := chi.URLParam(req, "id")
		if p.Name == "" && id == "" {
			p.Name = p.ID
		}
		if id != "" {
			var old cluster.Peer
			if err := db.QueryRowContext(req.Context(), "SELECT id,name,url,api_key FROM panel_nodes WHERE id=?", id).Scan(&old.ID, &old.Name, &old.URL, &old.Key); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					httpx.Fail(w, 404, "Сервер не найден")
				} else {
					httpx.Fail(w, 500, "Не удалось прочитать сервер")
				}
				return
			}
			p.ID = id
			if p.Name == "" {
				p.Name = old.Name
			}
			if p.Key == "" && p.URL == old.URL {
				p.Key = old.Key
			}
		}
		if id == "" && (len(p.ID) != 36 || p.Name == p.ID) {
			p.ID = store.UUID()
		}
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" || len(p.Name) > 120 {
			httpx.Fail(w, 400, "Укажите имя длиной от 1 до 120 байт")
			return
		}
		if _, ok := legacy[p.ID]; ok {
			httpx.Fail(w, 409, "Имя занято сервером из окружения")
			return
		}
		target, err := p.Validate()
		if err != nil {
			httpx.Fail(w, 400, err.Error())
			return
		}
		if id == "" {
			endpoint := "/api/status"
			if p.Key != "" {
				endpoint = "/node-api/status"
			}
			probe, err := http.NewRequestWithContext(req.Context(), http.MethodGet, target.String()+endpoint, nil)
			if err != nil {
				httpx.Fail(w, 400, "Некорректный адрес узла")
				return
			}
			if p.Key != "" {
				probe.Header.Set("Authorization", "Bearer "+p.Key)
			}
			client := &http.Client{Transport: cluster.Transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			response, err := client.Do(probe)
			if err != nil {
				httpx.Fail(w, 502, "Не удалось проверить идентичность узла")
				return
			}
			var identity struct {
				ID string `json:"node_id"`
			}
			err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&identity)
			response.Body.Close()
			if response.StatusCode != 200 || err != nil || !validation.SubID.MatchString(identity.ID) || len(identity.ID) != 36 {
				httpx.Fail(w, 502, "Узел не передал свой UUID")
				return
			}
			if input.Code != "" && len(p.ID) == 36 && p.ID != identity.ID {
				httpx.Fail(w, 409, "UUID узла не совпадает со строкой подключения")
				return
			}
			p.ID = identity.ID
			var own string
			if err := db.QueryRowContext(req.Context(), "SELECT uuid FROM node_identity WHERE singleton=1").Scan(&own); err != nil {
				httpx.Fail(w, 500, "Не удалось прочитать UUID панели")
				return
			}
			if own == p.ID {
				httpx.Fail(w, 409, "Этот узел уже локальный")
				return
			}
		}
		if id == "" {
			result, err := db.ExecContext(req.Context(), "INSERT INTO panel_nodes(id,name,url,api_key) VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING", p.ID, p.Name, p.URL, p.Key)
			if err != nil {
				httpx.Fail(w, 500, "Не удалось добавить сервер")
				return
			}
			if count, _ := result.RowsAffected(); count == 0 {
				httpx.Fail(w, 409, "Сервер с таким именем уже есть")
				return
			}
		} else {
			tx, err := db.BeginTx(req.Context(), nil)
			if err != nil {
				httpx.Fail(w, 500, "Не удалось сохранить сервер")
				return
			}
			defer tx.Rollback()
			result, err := tx.ExecContext(req.Context(), "UPDATE panel_nodes SET name=?,url=?,api_key=? WHERE id=?", p.Name, p.URL, p.Key, id)
			if err != nil {
				httpx.Fail(w, 500, "Не удалось сохранить сервер")
				return
			}
			if count, _ := result.RowsAffected(); count == 0 {
				httpx.Fail(w, 404, "Сервер не найден")
				return
			}
			var origin string
			if err := tx.QueryRowContext(req.Context(), "SELECT uuid FROM node_identity WHERE singleton=1").Scan(&origin); err != nil {
				httpx.Fail(w, 500, "Не удалось сохранить сервер")
				return
			}
			body, _ := json.Marshal(map[string]any{"singleton": 1, "uuid": id, "name": p.Name, "url": p.URL})
			if _, err := tx.ExecContext(req.Context(), `INSERT INTO replication_changes(kind,owner,record_key,version,origin,body)
VALUES('node_identity',?,?,MAX(CAST((julianday('now')-2440587.5)*86400000 AS INTEGER),COALESCE((SELECT MAX(version)+1 FROM replication_changes),0)),?,?)
ON CONFLICT(kind,owner,record_key) DO UPDATE SET version=excluded.version,origin=excluded.origin,body=excluded.body`, id, id, origin, string(body)); err != nil {
				httpx.Fail(w, 500, "Не удалось записать изменение узла")
				return
			}
			if err := tx.Commit(); err != nil {
				httpx.Fail(w, 500, "Не удалось сохранить сервер")
				return
			}
		}
		httpx.Write(w, 200, map[string]string{"id": p.ID})
	}
	r.Post("/nodes", save)
	r.Put("/nodes/{id}", save)
	r.Delete("/nodes/{id}", func(w http.ResponseWriter, req *http.Request) {
		result, err := db.ExecContext(req.Context(), "DELETE FROM panel_nodes WHERE id=?", chi.URLParam(req, "id"))
		if err != nil {
			httpx.Fail(w, 500, "Не удалось удалить сервер")
			return
		}
		if count, _ := result.RowsAffected(); count == 0 {
			httpx.Fail(w, 404, "Сервер не найден")
			return
		}
		httpx.Write(w, 200, map[string]bool{"ok": true})
	})
	r.Handle("/nodes/{id}/{endpoint}", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id, endpoint := chi.URLParam(req, "id"), chi.URLParam(req, "endpoint")
		if !cluster.Operation(req.Method, endpoint) {
			http.NotFound(w, req)
			return
		}
		p, found := legacy[id]
		if !found {
			err := db.QueryRowContext(req.Context(), "SELECT id,url,api_key FROM panel_nodes WHERE id=?", id).Scan(&p.ID, &p.URL, &p.Key)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.NotFound(w, req)
				} else {
					httpx.Fail(w, 500, "Не удалось прочитать сервер")
				}
				return
			}
		}
		u, err := p.Validate()
		if err != nil {
			httpx.Fail(w, 502, "Неверные настройки узла")
			return
		}
		proxy := &httputil.ReverseProxy{
			Transport: cluster.Transport,
			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetURL(u)
				r.Out.URL.Path = u.Path + "/api/" + endpoint
				r.Out.URL.RawPath = ""
				r.Out.URL.RawQuery = ""
				r.Out.Header.Del("Cookie")
				r.Out.Header.Del("Authorization")
				if p.Key != "" {
					r.Out.URL.Path = u.Path + "/node-api/" + endpoint
					r.Out.Header.Set("Authorization", "Bearer "+p.Key)
				}
			},
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
				httpx.Fail(w, 502, "Узел недоступен")
			},
		}
		proxy.ServeHTTP(w, req)
	}))
}
