package update

import (
	"context"
	"errors"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const Repository = "alexeeeeeeey/VPNPanel"
const latestURL = "https://api.github.com/repos/" + Repository + "/releases/latest"
const maxBinarySize = 128 << 20

type Status struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	Available   bool   `json:"available"`
	Installable bool   `json:"installable"`
	Updating    bool   `json:"updating"`
	LastChecked int64  `json:"last_checked"`
	Error       string `json:"error"`
}
type Manager struct {
	mu         sync.Mutex
	checking   sync.Mutex
	jobs       sync.WaitGroup
	status     Status
	client     *http.Client
	executable string
	platform   string
	arch       string
	verify     func(context.Context, string, string) error
	restart    chan struct{}
}

func New(version, executable string) *Manager {
	return &Manager{
		status: Status{Current: version, Installable: runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64")},
		client: &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" {
				return errors.New("недопустимое перенаправление загрузки")
			}
			return nil
		}},
		executable: executable, platform: runtime.GOOS, arch: runtime.GOARCH,
		verify: verifyBinary, restart: make(chan struct{}, 1),
	}
}
func (m *Manager) Status() Status           { m.mu.Lock(); defer m.mu.Unlock(); return m.status }
func (m *Manager) Restart() <-chan struct{} { return m.restart }

func (m *Manager) Wait() { m.jobs.Wait() }

// Run automatically installs stable releases at startup and every six hours.
func (m *Manager) Run(ctx context.Context) {
	if state := m.Status(); !stable(state.Current) || !state.Installable {
		return
	}
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		if err := m.Install(ctx); err != nil && ctx.Err() == nil {
			log.Printf("panel update: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func stable(version string) bool {
	v := "v" + strings.TrimPrefix(version, "v")
	return semver.IsValid(v) && semver.Prerelease(v) == ""
}
func newer(latest, current string) bool {
	left, right := "v"+strings.TrimPrefix(latest, "v"), "v"+strings.TrimPrefix(current, "v")
	return stable(latest) && semver.IsValid(right) && semver.Compare(left, right) > 0
}
func (m *Manager) Check(ctx context.Context) (Status, error) {
	m.checking.Lock()
	defer m.checking.Unlock()
	if state := m.Status(); state.LastChecked != 0 && time.Since(time.Unix(state.LastChecked, 0)) < time.Minute {
		if state.Error != "" {
			return state, errors.New(state.Error)
		}
		return state, nil
	}
	r, err := m.latest(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.LastChecked = time.Now().Unix()
	m.status.Error = ""
	if err != nil {
		m.status.Error = err.Error()
		return m.status, err
	}
	m.status.Latest = r.Tag
	m.status.Available = newer(r.Tag, m.status.Current)
	return m.status, nil
}
func (m *Manager) claim() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Updating || !m.status.Installable {
		return false
	}
	m.status.Updating = true
	m.status.Error = ""
	return true
}

// Start uses the application context so returning an HTTP response cannot cancel installation.
func (m *Manager) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !m.claim() {
		return errors.New("обновление уже выполняется или недоступно для этой ОС")
	}
	m.jobs.Add(1)
	go func() {
		defer m.jobs.Done()
		_ = m.installClaimed(ctx)
	}()
	return nil
}
func (m *Manager) Install(ctx context.Context) error {
	if !m.claim() {
		return errors.New("обновление уже выполняется или недоступно для этой ОС")
	}
	return m.installClaimed(ctx)
}
