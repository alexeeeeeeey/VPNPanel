package xray

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"vpnpanel/internal/assignment"
	"vpnpanel/internal/store"
)

type Status struct {
	State      string   `json:"state"`
	Error      string   `json:"error,omitempty"`
	TemplateID string   `json:"template_id"`
	Missing    []string `json:"missing"`
}
type Runtime struct {
	mu     sync.Mutex
	status Status
}

func (runtime *Runtime) Status() Status {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.status
}

func NewRuntime() *Runtime {
	return &Runtime{status: Status{State: "not_configured", Missing: []string{}}}
}

func (runtime *Runtime) Run(ctx context.Context, s *store.Store, binary, dir string) {
	releaseThread := bindXraySupervisor()
	defer releaseThread()
	var process *exec.Cmd
	var exited chan error
	var activeHash [32]byte
	var activePath string
	stop := func() {
		if process != nil {
			_ = process.Process.Kill()
			<-exited
			process = nil
		}
	}
	defer func() {
		stop()
		if activePath != "" {
			_ = os.Remove(activePath)
		}
	}()
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		state, body := assignment.ActiveConfig(ctx, s)
		status := Status{State: "unassigned", TemplateID: state.TemplateID, Missing: state.Missing, Error: state.Error}
		if process != nil {
			select {
			case err := <-exited:
				process = nil
				status.Error = fmt.Sprintf("Xray завершился: %v", err)
			default:
			}
		}
		switch {
		case state.TemplateID == "":
			stop()
		case !state.Ready:
			stop()
			status.State = "waiting"
		case binary == "":
			status.State = "not_configured"
		default:
			hash := sha256.Sum256(body)
			if hash == activeHash && process != nil {
				status.State = "running"
			} else {
				path, err := validateXray(ctx, binary, dir, body)
				if err != nil {
					stop()
					status.State = "error"
					status.Error = err.Error()
				} else {
					previousPath := activePath
					stop()
					command := exec.CommandContext(ctx, binary, "run", "-config", path)
					configureManagedXray(command)
					if err = command.Start(); err != nil {
						status.State = "error"
						status.Error = err.Error()
						_ = os.Remove(path)
					} else {
						process = command
						exited = make(chan error, 1)
						done := exited
						go func() { done <- command.Wait() }()
						activePath, activeHash = path, hash
						status.State = "running"
						if previousPath != "" {
							_ = os.Remove(previousPath)
						}
					}
				}
			}
		}
		runtime.mu.Lock()
		runtime.status = status
		runtime.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func validateXray(ctx context.Context, binary, dir string, body []byte) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, "xray-*.json")
	if err != nil {
		return "", err
	}
	path, _ := filepath.Abs(file.Name())
	if _, err = file.Write(body); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err = file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	validation, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(validation, binary, "run", "-test", "-config", path).CombinedOutput(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("Проверка Xray: %s (%w)", string(output), err)
	}
	return path, nil
}
