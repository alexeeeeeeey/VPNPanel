package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"vpnpanel/internal/store"
)

func Run(ctx context.Context, s *store.Store) {

	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			synchronizePeers(ctx, s)
		}
	}

}
func synchronizePeers(ctx context.Context, s *store.Store) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,name,url,api_key FROM panel_nodes UNION ALL SELECT id,name,url,api_key FROM replication_contacts WHERE id NOT IN (SELECT id FROM panel_nodes)")
	if err != nil {
		return
	}
	peers := []Peer{}
	for rows.Next() {
		var p Peer
		if rows.Scan(&p.ID, &p.Name, &p.URL, &p.Key) != nil {
			rows.Close()
			return
		}
		peers = append(peers, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	for _, p := range peers {
		if ctx.Err() != nil {
			return
		}
		if _, err := p.Validate(); err != nil {
			continue
		}
		exchangePeer(ctx, s, p)
	}
}

func exchangePeer(ctx context.Context, s *store.Store, p Peer) {
	client := &http.Client{Transport: Transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	endpoint := strings.TrimRight(p.URL, "/") + "/api/sync"
	if p.Key != "" {
		endpoint = strings.TrimRight(p.URL, "/") + "/node-api/sync"
	}
	request := func(method string, body []byte) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if p.Key != "" {
			req.Header.Set("Authorization", "Bearer "+p.Key)
		}
		req.Header.Set("X-Panel-Request", "1")
		req.Header.Set("Content-Type", "application/json")
		return client.Do(req)
	}
	response, err := request("GET", nil)
	if err != nil {
		return
	}
	var remote []store.Change
	err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&remote)
	response.Body.Close()
	if response.StatusCode != 200 || err != nil {
		return
	}
	if err = s.Merge(ctx, remote); err != nil {
		return
	}
	changes, err := s.Changes(ctx)
	if err != nil {
		return
	}
	raw, err := json.Marshal(changes)
	if err != nil || len(raw) > 2<<20 {
		return
	}
	response, err = request("POST", raw)
	if err != nil {
		return
	}
	response.Body.Close()
	if response.StatusCode == http.StatusOK {
		_, _ = s.DB.ExecContext(ctx, "DELETE FROM replication_contacts WHERE id=? AND id NOT IN (SELECT id FROM panel_nodes)", p.ID)
	}
}
