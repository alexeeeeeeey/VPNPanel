package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"vpnpanel/internal/config"
	"vpnpanel/internal/store"
	"vpnpanel/internal/subscription"
)

func TestUserTemplateWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	r := testRoutes(t.Context(), s)
	request := func(method, path, body string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("X-Panel-Request", "1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: status %d, body %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	var user config.User
	if err := json.Unmarshal(request("POST", "/api/users", `{"name":"Петя","sub_id":"legacy123abc456xy"}`, 201), &user); err != nil {
		t.Fatal(err)
	}
	if user.UUID == user.LimitedUUID || len(user.UUID) != 36 || len(user.LimitedUUID) != 36 {
		t.Fatal("expected two distinct UUIDs")
	}
	request("POST", "/api/users", `{"name":"Дубликат","sub_id":"legacy123abc456xy"}`, 409)
	request("PUT", "/api/variables", `{"body":"{\"PUBLIC_PORT\":443}"}`, 200)
	request("PUT", "/api/variables", `{"body":"{\"UUID\":\"overridden\"}"}`, 400)
	request("POST", "/api/templates", `{"name":"CDN","kind":"client","body":"{\"id\":\"&UUID_LIMITED\",\"port\":\"&PUBLIC_PORT\"}"}`, 200)
	sub := subscription.New(s, subscription.Settings{Enabled: true, Path: "/sub", Title: "VPN"})
	request("POST", "/api/templates", `{"name":"Direct","kind":"client","body":"{\"id\":\"&UUID\"}"}`, 200)
	request("POST", "/api/templates", `{"name":"Inbound","kind":"server","body":"{\"clients\":\"&CLIENTS_MAIN\"}"}`, 200)
	readSubscription := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/sub/"+user.SubID, nil)
		req.Header.Set("X-HWID", "test-device")
		req.Header.Set("X-Device-OS", "Android")
		req.Header.Set("X-Device-Model", "Test phone")
		req.Header.Set("User-Agent", "Happ/test")
		sub.ServeHTTP(w, req)
		return w
	}
	w := readSubscription()
	var profiles []struct {
		ID   string
		Port int
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &profiles) != nil || len(profiles) != 2 ||
		profiles[0].ID != user.LimitedUUID || profiles[0].Port != 443 || profiles[1].ID != user.UUID || w.Header().Get("profile-title") != "base64:VlBO" {
		t.Fatalf("invalid subscription response: %d", w.Code)
	}
	activity, err := s.UserActivity(t.Context(), user.ID)
	if err != nil || len(activity.Devices) != 1 || activity.Devices[0].HWID != "test-device" || activity.Devices[0].Model != "Test phone" || activity.SubscriptionLastSeenAt == 0 {
		t.Fatal("subscription device metadata not captured", err)
	}
	library, err := s.Templates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var cdn, direct, serverID string
	for _, template := range library {
		switch template.Name {
		case "CDN":
			cdn = template.ID
		case "Direct":
			direct = template.ID
		case "Inbound":
			serverID = template.ID
		}
	}
	orderBody, _ := json.Marshal(map[string]any{"kind": "client", "ids": []string{direct, cdn}})
	request("PUT", "/api/template-order", string(orderBody), 200)
	w = readSubscription()
	if json.Unmarshal(w.Body.Bytes(), &profiles) != nil || profiles[0].ID != user.UUID || profiles[1].ID != user.LimitedUUID {
		t.Fatal("subscription ignored manual client ordering")
	}
	invalidOrder, _ := json.Marshal(map[string]any{"kind": "client", "ids": []string{direct, serverID}})
	request("PUT", "/api/template-order", string(invalidOrder), 409)
	body, _ := json.Marshal(map[string]string{"kind": "client", "body": `{"id":"&UUID_LIMITED"}`, "user_id": user.ID})
	if !bytes.Contains(request("POST", "/api/render", string(body), 200), []byte(user.LimitedUUID)) {
		t.Fatal("wrong credential")
	}
	request("PATCH", "/api/users/"+user.ID, `{"expires_at":1}`, 200)
	if w := readSubscription(); w.Code != 404 {
		t.Fatalf("expired user can fetch subscription: %d", w.Code)
	}
	request("POST", "/api/render", string(body), 400)
	request("PATCH", "/api/users/"+user.ID, `{"expires_at":0}`, 200)
	if w := readSubscription(); w.Code != 200 {
		t.Fatalf("renewed user cannot fetch subscription: %d", w.Code)
	}
	request("PATCH", "/api/users/"+user.ID, `{"enabled":false}`, 200)
	if w := readSubscription(); w.Code != 404 {
		t.Fatalf("disabled user can fetch subscription: %d", w.Code)
	}
	request("POST", "/api/render", string(body), 400)
	server := request("POST", "/api/render", `{"kind":"server","body":"{\"clients\":\"&CLIENTS_MAIN\"}"}`, 200)
	if bytes.Contains(server, []byte(user.UUID)) {
		t.Fatal("disabled credential remains in server config")
	}
	request("POST", "/api/users", `{"name":"bad","uuid":"injected"}`, 400)
	if err := s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r = testRoutes(t.Context(), s)
	if !bytes.Contains(request("GET", "/api/state", "", 200), []byte(user.LimitedUUID)) {
		t.Fatal("credentials lost after reopening database")
	}
	request("PATCH", "/api/users/"+user.ID, `{"reset_main":true}`, 200)
	request("PATCH", "/api/users/"+user.ID, `{"limited_bytes":-1}`, 400)
	request("PATCH", "/api/users/"+user.ID, `{"limited_bytes":1073741824}`, 200)
	list, err := s.Users(t.Context())
	if err != nil || len(list) != 1 || list[0].UUID == user.UUID || list[0].LimitedUUID != user.LimitedUUID || list[0].SubID != user.SubID {
		t.Fatalf("MAIN reset did not isolate the selected credential: %v", err)
	}
	if user.CreatedAt <= 0 || list[0].CreatedAt != user.CreatedAt || list[0].LimitedBytes != 1073741824 {
		t.Fatal("creation timestamp or limited traffic quota was not stored correctly")
	}
	mainUUID := list[0].UUID
	request("PATCH", "/api/users/"+user.ID, `{"name":"  Новое имя  "}`, 200)
	request("PATCH", "/api/users/"+user.ID, `{"name":"  "}`, 400)
	request("PATCH", "/api/users/"+user.ID, `{"reset_limited":true}`, 200)
	list, err = s.Users(t.Context())
	if err != nil || len(list) != 1 || list[0].UUID != mainUUID || list[0].LimitedUUID == user.LimitedUUID || list[0].SubID != user.SubID {
		t.Fatalf("LIMITED reset did not isolate the selected credential: %v", err)
	}
	if list[0].Name != "Новое имя" {
		t.Fatal("user rename was not persisted")
	}
	request("PATCH", "/api/users/"+user.ID, `{"enabled":true}`, 200)
	request("DELETE", "/api/users/"+user.ID, "", 200)
	request("DELETE", "/api/users/"+user.ID, "", 404)
	if w := readSubscription(); w.Code != 404 {
		t.Fatalf("deleted user can fetch subscription: %d", w.Code)
	}
	list, err = s.Users(t.Context())
	if err != nil || len(list) != 0 {
		t.Fatalf("deleted user remains in storage: %v", err)
	}
}
