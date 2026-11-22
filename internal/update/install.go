package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) installClaimed(ctx context.Context) (err error) {
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.status.Error = err.Error()
		}
		if err != nil || !m.status.Available {
			m.status.Updating = false
		}
	}()
	r, err := m.latest(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.status.Latest = r.Tag
	m.status.Available = newer(r.Tag, m.status.Current)
	m.status.LastChecked = time.Now().Unix()
	available := m.status.Available
	m.mu.Unlock()
	if !available {
		return nil
	}
	name := "vpnpanel-" + m.platform + "-" + m.arch
	binaryURL, checksumURL := "", ""
	for _, a := range r.Assets {
		switch a.Name {
		case name:
			binaryURL = a.URL
		case "SHA256SUMS":
			checksumURL = a.URL
		}
	}
	if binaryURL == "" || checksumURL == "" {
		return fmt.Errorf("в релизе %s отсутствует %s или SHA256SUMS", r.Tag, name)
	}
	sums, err := m.download(ctx, checksumURL, 64<<10)
	if err != nil {
		return err
	}
	expected, err := checksum(sums, name)
	if err != nil {
		return err
	}
	data, err := m.download(ctx, binaryURL, maxBinarySize)
	if err != nil {
		return err
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != expected {
		return errors.New("SHA-256 скачанного бинарника не совпадает")
	}
	executable, err := filepath.EvalSymlinks(m.executable)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(executable), ".vpnpanel-update-*")
	if err != nil {
		return fmt.Errorf("каталог бинарника недоступен для записи: %w", err)
	}
	staged := file.Name()
	defer os.Remove(staged)
	if _, err = file.Write(data); err == nil {
		err = file.Chmod(0755)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = m.verify(ctx, staged, r.Tag); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(staged, executable); err != nil {
		return fmt.Errorf("не удалось заменить бинарник: %w", err)
	}
	m.restart <- struct{}{}
	return nil
}
func verifyBinary(ctx context.Context, path, version string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "-version").Output()
	if err != nil || strings.TrimSpace(string(output)) != version {
		return fmt.Errorf("скачанный бинарник не подтвердил версию %s", version)
	}
	return nil
}
