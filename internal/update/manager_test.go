package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixture(t *testing.T, version string, validChecksum bool) (*Manager, string) {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "vpnpanel")
	if err := os.WriteFile(executable, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	m := New(version, executable)
	m.platform, m.arch = "linux", "amd64"
	m.status.Installable = true
	binary := []byte("new binary")
	hash := sha256.Sum256(binary)
	digest := hex.EncodeToString(hash[:])
	if !validChecksum {
		digest = strings.Repeat("0", 64)
	}
	base := "https://github.com/" + Repository + "/releases/download/v1.1.0/"
	r := release{Tag: "v1.1.0", Assets: []asset{{Name: "vpnpanel-linux-amd64", URL: base + "vpnpanel-linux-amd64"}, {Name: "SHA256SUMS", URL: base + "SHA256SUMS"}}}
	raw, _ := json.Marshal(r)
	m.client = &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		var body string
		switch req.URL.String() {
		case latestURL:
			body = string(raw)
		case base + "SHA256SUMS":
			body = fmt.Sprintf("%s  vpnpanel-linux-amd64\n", digest)
		case base + "vpnpanel-linux-amd64":
			body = string(binary)
		default:
			t.Errorf("unexpected URL: %s", req.URL)
			body = ""
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	m.verify = func(ctx context.Context, path, version string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != "new binary" || version != "v1.1.0" {
			return fmt.Errorf("unexpected staged binary")
		}
		return nil
	}
	return m, executable
}

func TestInstallVerifiesAndReplacesBinary(t *testing.T) {
	m, executable := fixture(t, "v1.0.0", true)
	if err := m.Install(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil || string(data) != "new binary" {
		t.Fatalf("binary not replaced: %s, %v", data, err)
	}
	select {
	case <-m.Restart():
	default:
		t.Fatal("restart not requested")
	}
	if !m.Status().Updating {
		t.Fatal("installation must stay locked until restart")
	}
	if err := m.Start(t.Context()); err == nil {
		t.Fatal("duplicate installation accepted")
	}
}
func TestFailedVerificationPreservesRunningBinary(t *testing.T) {
	for _, failure := range []string{"checksum", "version"} {
		t.Run(failure, func(t *testing.T) {
			m, executable := fixture(t, "v1.0.0", failure != "checksum")
			if failure == "version" {
				m.verify = func(context.Context, string, string) error { return fmt.Errorf("invalid executable") }
			}
			if err := m.Install(t.Context()); err == nil {
				t.Fatal("invalid release installed")
			}
			data, _ := os.ReadFile(executable)
			if string(data) != "old binary" {
				t.Fatal("running binary overwritten")
			}
			select {
			case <-m.Restart():
				t.Fatal("restart after failure")
			default:
			}
			if m.Status().Updating || m.Status().Error == "" {
				t.Fatal("failure not recorded")
			}
		})
	}
}
func TestNoDowngrade(t *testing.T) {
	for _, version := range []string{"v1.1.0", "v2.0.0"} {
		m, executable := fixture(t, version, true)
		if err := m.Install(t.Context()); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(executable)
		if string(data) != "old binary" || m.Status().Updating {
			t.Fatal("equal or older release installed")
		}
	}
}
func TestMissingRepositoryAndCheckCache(t *testing.T) {
	m := New("v1.0.0", "")
	calls := 0
	m.client = &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	for i := 0; i < 2; i++ {
		state, err := m.Check(t.Context())
		if err != nil || state.Available || state.Latest != "" {
			t.Fatalf("unexpected state: %+v %v", state, err)
		}
	}
	if calls != 1 {
		t.Fatalf("cache missed: %d", calls)
	}
}
func TestOnlyStableNewerVersions(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.0", "v1.1.0", true}, {"1.2.0", "1.1.0", true},
		{"v1.2.0-rc.1", "v1.1.0", false}, {"v1.1.0", "v1.1.0", false},
		{"garbage", "v1.1.0", false}, {"v1.0.0", "v2.0.0", false},
	} {
		if got := newer(tc.latest, tc.current); got != tc.want {
			t.Fatalf("newer(%q,%q)=%v", tc.latest, tc.current, got)
		}
	}
}
func TestRejectExternalAssetURL(t *testing.T) {
	m := New("v1.0.0", "")
	for _, address := range []string{"http://github.com/" + Repository + "/releases/download/v1/a", "https://example.com/a", "https://github.com/another/repo/releases/download/v1/a"} {
		if _, err := m.download(t.Context(), address, 100); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
}

func TestCancellationPreservesRunningBinary(t *testing.T) {
	m, executable := fixture(t, "v1.0.0", true)
	ctx, cancel := context.WithCancel(t.Context())
	m.verify = func(context.Context, string, string) error {
		cancel()
		return nil
	}
	if err := m.Install(ctx); err == nil {
		t.Fatal("cancelled installation completed")
	}
	data, _ := os.ReadFile(executable)
	if string(data) != "old binary" {
		t.Fatal("binary replaced during shutdown")
	}
	entries, err := os.ReadDir(filepath.Dir(executable))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary update not cleaned up: %v, %v", entries, err)
	}
}
