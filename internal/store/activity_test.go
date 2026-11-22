package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestActivityCountersAreIdempotentAndMonotonic(t *testing.T) {
	open := func(name string) *Store {
		s, err := Open(filepath.Join(t.TempDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.DB.Close() })
		return s
	}
	a, b := open("a.db"), open("b.db")
	now := time.Now()
	add := func(s *Store, main, limited int64) {
		if err := s.AddTraffic(t.Context(), now, []TrafficDelta{{UserID: "user", MainDown: main, LimitedDown: limited}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	add(a, 100, 200)
	stale, err := a.Changes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Merge(t.Context(), stale); err != nil {
		t.Fatal(err)
	}
	add(a, 20, 30)
	fresh, err := a.Changes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, changes := range [][]Change{fresh, fresh, stale} {
		if err = b.Merge(t.Context(), changes); err != nil {
			t.Fatal(err)
		}
	}
	add(b, 5, 7)
	changes, err := b.Changes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Merge(t.Context(), changes); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Store{a, b} {
		summaries, err := s.ActivitySummaries(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		value := summaries["user"]
		if value.TotalMain != 125 || value.TotalLimited != 237 || value.QuotaLimited != 237 {
			t.Fatalf("wrong usage totals: %+v", value)
		}
	}
	previous := now.AddDate(0, -1, 0)
	if err = a.AddTraffic(t.Context(), previous, []TrafficDelta{{UserID: "user", LimitedDown: 50}}, nil); err != nil {
		t.Fatal(err)
	}
	summaries, err := a.ActivitySummaries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if value := summaries["user"]; value.TotalLimited != 287 || value.QuotaLimited != 237 {
		t.Fatalf("quota includes other month: %+v", value)
	}
}

func TestSubscriptionDevicesMergeAcrossNodes(t *testing.T) {
	a, err := Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	b, err := Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.DB.Close()
	now := time.Now()
	for _, s := range []*Store{a, b} {
		if err = s.RecordSubscription(t.Context(), "user", now, SubscriptionDevice{HWID: "device-1", UserAgent: "Happ/1", OS: "Android"}); err != nil {
			t.Fatal(err)
		}
		if err = s.RecordSubscription(t.Context(), "user", now, SubscriptionDevice{UserAgent: "Other/1"}); err != nil {
			t.Fatal(err)
		}
	}
	changes, err := a.Changes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Merge(t.Context(), changes); err != nil {
		t.Fatal(err)
	}
	if err = b.Merge(t.Context(), changes); err != nil {
		t.Fatal(err)
	}
	activity, err := b.UserActivity(t.Context(), "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.Devices) != 1 || len(activity.WithoutHWID) != 1 || activity.DeviceCount != 1 || activity.Devices[0].RequestCount != 2 {
		t.Fatalf("devices counted incorrectly: %+v", activity)
	}
	if activity.WithoutHWID[0].RequestCount != 2 {
		t.Fatal("requests without HWID counted incorrectly")
	}
}
