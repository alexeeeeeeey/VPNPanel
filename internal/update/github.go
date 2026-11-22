package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}
type release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

func (m *Manager) latest(ctx context.Context) (release, error) {
	query, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(query, http.MethodGet, latestURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "VPNPanel")
	response, err := m.client.Do(req)
	if err != nil {
		return release{}, fmt.Errorf("не удалось проверить релизы: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return release{}, nil
	}
	if response.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("GitHub Releases: HTTP %d", response.StatusCode)
	}
	var r release
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&r); err != nil {
		return release{}, err
	}
	if r.Draft || r.Prerelease || !stable(r.Tag) {
		return release{}, errors.New("релиз не является стабильной версией SemVer")
	}
	return r, nil
}
func (m *Manager) download(ctx context.Context, address string, limit int64) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || !strings.HasPrefix(u.Path, "/"+Repository+"/releases/download/") {
		return nil, errors.New("недопустимый адрес файла релиза")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("загрузка релиза: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err == nil && int64(len(data)) > limit {
		return nil, errors.New("файл релиза превышает допустимый размер")
	}
	return data, err
}
func checksum(data []byte, name string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		raw, err := hex.DecodeString(fields[0])
		if err != nil || len(raw) != sha256.Size {
			break
		}
		return strings.ToLower(fields[0]), nil
	}
	return "", fmt.Errorf("в SHA256SUMS нет корректной суммы %s", name)
}
