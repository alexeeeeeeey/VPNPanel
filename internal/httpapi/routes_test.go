package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"vpnpanel/internal/metrics"
	"vpnpanel/internal/store"
	"vpnpanel/internal/subscription"
	"vpnpanel/internal/update"
	"vpnpanel/internal/xray"
)

func testRoutes(ctx context.Context, s *store.Store) *chi.Mux {
	return New(Dependencies{Store: s, Subscriptions: subscription.Start(ctx, s),
		Sample: metrics.NewSampler(ctx, ".", s), Runtime: xray.NewRuntime(), Collector: &xray.Collector{},
		Context: ctx, Updater: update.New("0.1.0-dev", "")})
}

func TestUpdateStatusDoesNotRequireRepositoryConfiguration(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.DB.Close() })
	w := httptest.NewRecorder()
	testRoutes(t.Context(), s).ServeHTTP(w, httptest.NewRequest("GET", "/api/update-status", nil))
	var state update.Status
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &state) != nil || state.Current != "0.1.0-dev" {
		t.Fatalf("update status unavailable: %d %s", w.Code, w.Body.String())
	}
}
