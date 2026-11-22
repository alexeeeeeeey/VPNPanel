package subscription

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"vpnpanel/internal/config"
	"vpnpanel/internal/store"
	"vpnpanel/internal/validation"
)

func (sub *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sub.mu.Lock()
	settings := sub.settings
	sub.mu.Unlock()
	id, ok := strings.CutPrefix(r.URL.Path, settings.Path+"/")
	if !settings.Enabled || !ok || !validation.SubID.MatchString(id) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	var user config.User
	err := sub.store.DB.QueryRowContext(r.Context(), "SELECT id,name,uuid,limited_uuid,enabled,expires_at FROM users WHERE sub_id=?", id).Scan(&user.ID, &user.Name, &user.UUID, &user.LimitedUUID, &user.Enabled, &user.ExpiresAt)
	if err != nil || !user.Active(time.Now()) {
		http.NotFound(w, r)
		return
	}
	changes, err := sub.store.Changes(r.Context())
	if err != nil {
		http.Error(w, "Subscription unavailable", http.StatusServiceUnavailable)
		return
	}
	variables := map[string]map[string]any{}
	identity, err := sub.store.Identity()
	if err != nil {
		http.Error(w, "Subscription unavailable", 503)
		return
	}
	variables[identity.ID] = map[string]any{}
	for _, change := range changes {
		if change.Table != "panel_variables" || string(change.Data) == "null" {
			continue
		}
		var row struct{ Body string }
		if json.Unmarshal(change.Data, &row) != nil {
			http.Error(w, "Subscription unavailable", 503)
			return
		}
		values, err := config.Decode(row.Body)
		if err != nil {
			http.Error(w, "Subscription unavailable", 503)
			return
		}
		variables[change.Owner] = values
	}
	configs := []json.RawMessage{}
	order := map[string]int{}
	for _, change := range changes {
		if change.Table != "template_order" || change.Key != "client" || string(change.Data) == "null" {
			continue
		}
		var row struct{ Body string }
		var ids []string
		if json.Unmarshal(change.Data, &row) == nil && json.Unmarshal([]byte(row.Body), &ids) == nil {
			for index, id := range ids {
				order[id] = index + 1
			}
		}
	}
	owners := []string{}
	for owner := range variables {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	seen := map[string]bool{}
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Table != "templates" || changes[j].Table != "templates" {
			return changes[i].Table < changes[j].Table
		}
		var left, right store.Template
		_ = json.Unmarshal(changes[i].Data, &left)
		_ = json.Unmarshal(changes[j].Data, &right)
		leftRank, rightRank := order[left.ID], order[right.ID]
		if leftRank == 0 {
			leftRank = int(^uint(0) >> 1)
		}
		if rightRank == 0 {
			rightRank = int(^uint(0) >> 1)
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if changes[i].Owner != changes[j].Owner {
			return changes[i].Owner < changes[j].Owner
		}
		return changes[i].Key < changes[j].Key
	})
	for _, change := range changes {
		if change.Table != "templates" || string(change.Data) == "null" {
			continue
		}
		var template store.Template
		if json.Unmarshal(change.Data, &template) != nil {
			http.Error(w, "Subscription unavailable", 503)
			return
		}
		if template.Kind != "client" {
			continue
		}
		for _, owner := range owners {
			missing, err := config.Missing(template.Body, variables[owner])
			if err != nil || len(missing) > 0 {
				continue
			}
			rendered, err := config.RenderWithVariables(template.Body, "client", nil, &user, variables[owner])
			if err != nil {
				continue
			}
			if !seen[string(rendered)] {
				configs = append(configs, rendered)
				seen[string(rendered)] = true
			}
		}
	}
	headers := w.Header()
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("Cache-Control", "private, no-store")
	if settings.Title != "" {
		headers.Set("profile-title", "base64:"+base64.StdEncoding.EncodeToString([]byte(settings.Title)))
	}
	if settings.Description != "" {
		headers.Set("announce", "base64:"+base64.StdEncoding.EncodeToString([]byte(settings.Description)))
	}
	if settings.Support != "" {
		headers.Set("support-url", settings.Support)
	}
	if settings.Website != "" {
		headers.Set("profile-web-page-url", settings.Website)
	}
	if settings.Interval > 0 {
		headers.Set("profile-update-interval", strconv.Itoa(settings.Interval))
	}
	if r.Method == http.MethodGet {
		if err := recordSubscription(sub.store, r, user.ID); err != nil {
			http.Error(w, "Не удалось сохранить обращение к подписке", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(configs)
	}
}
