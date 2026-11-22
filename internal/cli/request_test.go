package cli

import (
	"encoding/json"
	"testing"
)

func TestUserCommands(t *testing.T) {
	const id = "fa6bd25b-5ec7-4a04-afb3-e02cd943e980"
	for _, tc := range []struct {
		command string
		enabled bool
	}{
		{"user-enable", true},
		{"user-disable", false},
	} {
		t.Run(tc.command, func(t *testing.T) {
			method, path, payload, err := request([]string{tc.command, id})
			if err != nil {
				t.Fatal(err)
			}
			var body struct{ Enabled bool }
			if err := json.Unmarshal(payload, &body); err != nil {
				t.Fatal(err)
			}
			if method != "PATCH" || path != "users/"+id || body.Enabled != tc.enabled {
				t.Fatalf("unexpected request: %s %s %s", method, path, payload)
			}
		})
	}
}

func TestInvalidCommandsDoNotReachSSH(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"unknown"},
		{"user-disable", "invalid;id"},
		{"state", "extra"},
		{"render", "client", "unused.json"},
	} {
		if err := Execute("test-host", 9080, "/panel", "/etc/vpnpanel/cli.headers", args); err == nil {
			t.Fatalf("accepted invalid command: %q", args)
		}
	}
}
