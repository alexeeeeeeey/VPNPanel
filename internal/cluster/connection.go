package cluster

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
)

func ConnectionCode(id, name, address, prefix string) (string, error) {
	p := Peer{ID: id, Name: name, URL: address, Key: os.Getenv("VPNPANEL_NODE_KEY")}
	u, err := p.Validate()
	if err != nil || len(p.Key) < 32 || (u != nil && u.Path != prefix) {
		return "", fmt.Errorf("connection URL must include the configured base path and VPNPANEL_NODE_KEY must be set")
	}
	raw, _ := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
