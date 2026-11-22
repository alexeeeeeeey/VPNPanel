package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDollarVariablesAndSystemUserIdentity(t *testing.T) {
	u := User{UUID: "main", LimitedUUID: "limited", Enabled: true}
	raw := `{"id":"$USER_UUID","port":$PORT,"settings":$SETTINGS}`
	values := map[string]any{"PORT": 443, "SETTINGS": map[string]any{"enabled": true}}
	out, err := RenderWithVariables(raw, "client", nil, &u, values)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ID       string
		Port     int
		Settings struct{ Enabled bool }
	}
	if json.Unmarshal(out, &result) != nil || result.ID != "main" || result.Port != 443 || !result.Settings.Enabled {
		t.Fatalf("wrong substitution: %s", out)
	}
	missing, err := Missing(raw, map[string]any{"PORT": 0})
	if err != nil || len(missing) != 1 || missing[0] != "SETTINGS" {
		t.Fatal("wrong readiness check", missing, err)
	}
}

func TestTextSubstitutionUsesTemplateQuotesAndRejectsInvalidResult(t *testing.T) {
	u := User{Enabled: true, UUID: "main"}
	raw := `{"port":$PORT,"label":"port-$PORT","url":"https://$HOST:$PORT"}`
	out, err := RenderWithVariables(raw, "client", nil, &u, map[string]any{"PORT": "443", "HOST": "vpn.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Port  int
		Label string
		URL   string
	}
	if json.Unmarshal(out, &value) != nil || value.Port != 443 || value.Label != "port-443" || value.URL != "https://vpn.example.com:443" {
		t.Fatalf("unexpected result %s", out)
	}
	if _, err := RenderWithVariables(raw, "client", nil, &u, map[string]any{"PORT": "wrong", "HOST": "vpn.example.com"}); err == nil {
		t.Fatal("invalid rendered JSON accepted")
	}
	if _, err := Requirements(raw); err != nil {
		t.Fatal("template with unquoted markers rejected", err)
	}
}

func TestMonthlyQuotaOnlyRemovesLimitedIdentity(t *testing.T) {
	users := []User{{ID: "user", UUID: "main", LimitedUUID: "limited", Enabled: true, LimitedBytes: 100, QuotaLimited: 100}}
	out, err := Render(`{"main":"&CLIENTS_MAIN","limited":"&CLIENTS_LIMITED"}`, "server", users, nil)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Main    []any
		Limited []any
	}
	if json.Unmarshal(out, &result) != nil || len(result.Main) != 1 || len(result.Limited) != 0 {
		t.Fatalf("wrong quota enforcement: %s", out)
	}
	users[0].QuotaLimited = 99
	out, err = Render(`{"clients":"&CLIENTS_LIMITED"}`, "server", users, nil)
	if err != nil || !strings.Contains(string(out), `"limited"`) {
		t.Fatal("limited identity blocked below quota", err)
	}
}

func TestRenderIndependentCredentials(t *testing.T) {
	u := User{ID: "person", UUID: "main-id", LimitedUUID: "limited-id", Enabled: true}
	out, err := Render(`{"main":"&UUID","limited":"&UUID_LIMITED","address":"cdn.example.com","large":9007199254740993}`, "client", nil, &u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"main-id"`) || !strings.Contains(string(out), `"limited-id"`) || !strings.Contains(string(out), `9007199254740993`) || !strings.Contains(string(out), `cdn.example.com`) {
		t.Fatalf("unexpected output: %s", out)
	}
	users := []User{u, {ID: "disabled", UUID: "excluded", LimitedUUID: "also-excluded", Enabled: false}}
	out, err = Render(`{"main":"&CLIENTS_MAIN","limited":"&CLIENTS_LIMITED"}`, "server", users, nil)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Main []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		}
		Limited []struct {
			ID string `json:"id"`
		}
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Main) != 1 || len(result.Limited) != 1 || result.Main[0].ID != u.UUID || result.Limited[0].ID != u.LimitedUUID || result.Main[0].Email != "person.main" {
		t.Fatalf("incorrect account isolation: %s", out)
	}
}

func TestRejectInvalidTemplatesAndDisabledUser(t *testing.T) {
	u := User{Enabled: true}
	for _, raw := range []string{`[]`, `null`, `{} {}`, `{"id":"&UNKNOWN"}`, `{"id":"&CLIENTS_MAIN"}`, `{"&UUID":"value"}`} {
		if _, err := Render(raw, "client", nil, &u); err == nil {
			t.Errorf("accepted invalid template %s", raw)
		}
	}
	u.Enabled = false
	if _, err := Render(`{"id":"&UUID"}`, "client", nil, &u); err == nil {
		t.Fatal("disabled user received a config")
	}
	out, err := Render(`{"clients":"&CLIENTS_MAIN"}`, "server", nil, nil)
	if err != nil || !strings.Contains(string(out), `[]`) {
		t.Fatalf("empty clients must be an array: %s, %v", out, err)
	}
}
