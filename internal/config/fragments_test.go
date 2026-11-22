package config

import (
	"encoding/json"
	"strings"
	"testing"
)

const statsFragment = `"stats": {},
"api": {"listen": "127.0.0.1:10085", "services": ["StatsService"]},
"policy": {"levels": {"0": {"statsUserUplink": true, "statsUserDownlink": true, "statsUserOnline": true}}}`

func TestRawObjectFieldsAreValidatedAfterSubstitution(t *testing.T) {
	raw := `{"inbounds":[{"clients":$CLIENTS_MAIN}],$XRAY_STATS}`
	if err := ValidateTemplate(raw, "server"); err != nil {
		t.Fatal("raw fragment rejected before substitution:", err)
	}
	missing, err := Missing(raw, nil)
	if err != nil || len(missing) != 1 || missing[0] != "XRAY_STATS" {
		t.Fatalf("wrong requirements: %v %v", missing, err)
	}
	users := []User{{ID: "user", UUID: "main", Enabled: true}}
	out, err := RenderWithVariables(raw, "server", users, nil, map[string]any{"XRAY_STATS": statsFragment})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Stats map[string]any
		API   struct {
			Listen   string
			Services []string
		}
		Policy   struct{ Levels map[string]map[string]bool }
		Inbounds []struct{ Clients []struct{ ID string } }
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result.Stats == nil || result.API.Listen != "127.0.0.1:10085" || len(result.API.Services) != 1 ||
		!result.Policy.Levels["0"]["statsUserUplink"] || result.Inbounds[0].Clients[0].ID != "main" {
		t.Fatalf("fragment or generated users lost: %s", out)
	}
}

func TestRawFragmentsRejectInvalidFinalJSONAndDuplicatePolicy(t *testing.T) {
	for _, tc := range []struct{ raw, fragment string }{
		{`{"inbounds":[],$EXTRA}`, `"stats": {},`},
		{`{"inbounds":[],$EXTRA}`, `not-json`},
		{`{"inbounds":[],$EXTRA}`, `"api": {`},
		{`{"policy":{"system":{"statsInboundUplink":true}},$EXTRA}`, statsFragment},
	} {
		if _, err := RenderWithVariables(tc.raw, "server", nil, nil, map[string]any{"EXTRA": tc.fragment}); err == nil {
			t.Fatalf("invalid or conflicting fragment accepted: %s", tc.fragment)
		}
	}
	_, err := RenderWithVariables(`{"policy":{"foo":"bar"},$EXTRA}`, "server", nil, nil, map[string]any{"EXTRA": statsFragment})
	if err == nil || !strings.Contains(err.Error(), `"policy"`) {
		t.Fatalf("policy conflict not explained: %v", err)
	}
}

func TestRawArrayFragmentAndLegacyTypedMarkers(t *testing.T) {
	raw := `{"routes":[$ROUTES],"host":"&HOST"}`
	missing, err := Missing(raw, map[string]any{"ROUTES": `{"tag":"a"},{"tag":"b"}`})
	if err != nil || len(missing) != 1 || missing[0] != "HOST" {
		t.Fatalf("legacy marker lost: %v %v", missing, err)
	}
	out, err := RenderWithVariables(raw, "server", nil, nil, map[string]any{"ROUTES": `{"tag":"a"},{"tag":"b"}`, "HOST": "vpn.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Routes []map[string]string
		Host   string
	}
	if json.Unmarshal(out, &result) != nil || len(result.Routes) != 2 || result.Host != "vpn.example.com" {
		t.Fatalf("incorrect array fragment: %s", out)
	}
}
