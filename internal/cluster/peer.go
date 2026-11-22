package cluster

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"vpnpanel/internal/validation"
)

var nodeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,48}$`)
var Transport = &http.Transport{
	DialContext:           (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
	ResponseHeaderTimeout: 9 * time.Second,
	IdleConnTimeout:       30 * time.Second,
}

type Peer struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	URL  string `json:"url"`
	Key  string `json:"key,omitempty"`
}

func (p Peer) Validate() (*url.URL, error) {
	u, err := url.Parse(p.URL)
	if err != nil || !nodeID.MatchString(p.ID) || p.ID == "local" || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" ||
		(u.Path != "" && !validation.Path.MatchString(u.Path)) {
		return nil, errors.New("Укажите имя узла и URL панели с её базовым путём")
	}
	loopback := net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("Для удалённого узла нужен HTTPS или локальный SSH-туннель")
	}
	if (u.Scheme == "https" || p.Key != "") && (len(p.Key) < 32 || len(p.Key) > 256 || strings.ContainsAny(p.Key, " \t\r\n")) {
		return nil, errors.New("Нужен API-ключ длиной от 32 символов")
	}
	return u, nil
}

func Operation(method, endpoint string) bool {
	return (method == http.MethodGet && slices.Contains([]string{"status", "state", "update", "update-status", "variables", "assignment", "runtime", "sync"}, endpoint)) ||
		(method == http.MethodPost && slices.Contains([]string{"templates", "render", "sync", "update"}, endpoint)) ||
		(method == http.MethodPut && slices.Contains([]string{"variables", "assignment"}, endpoint))
}
