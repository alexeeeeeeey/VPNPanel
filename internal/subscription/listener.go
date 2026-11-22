package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"vpnpanel/internal/store"
)

type Service struct {
	store        *store.Store
	mu           sync.Mutex
	settings     Settings
	server       *http.Server
	startupError string
}

func (sub *Service) Apply(ctx context.Context, next Settings, persist bool) error {
	if err := next.validate(); err != nil {
		return err
	}
	sub.mu.Lock()
	defer sub.mu.Unlock()
	address := net.JoinHostPort(next.IP, strconv.Itoa(next.Port))
	var listener net.Listener
	if next.Enabled && (sub.server == nil || sub.server.Addr != address) {
		var err error
		listener, err = net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("Не удалось открыть %s: %w", address, err)
		}
	}
	if persist {
		raw, _ := json.Marshal(next)
		if _, err := sub.store.DB.ExecContext(ctx, "INSERT INTO subscription_settings(id, body) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", raw); err != nil {
			if listener != nil {
				_ = listener.Close()
			}
			return errors.New("Не удалось сохранить подписку")
		}
	}
	sub.settings = next
	sub.startupError = ""
	if sub.server != nil && (listener != nil || !next.Enabled) {
		_ = sub.server.Close()
		sub.server = nil
	}
	if listener != nil {
		server := &http.Server{Addr: address, Handler: http.HandlerFunc(sub.ServeHTTP), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
		sub.server = server
		go func() {
			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("subscription listener: %v", err)
			}
		}()
	}
	return nil
}

func New(s *store.Store, settings Settings) *Service { return &Service{store: s, settings: settings} }
func Start(ctx context.Context, s *store.Store) *Service {
	sub := New(s, Settings{IP: "127.0.0.1", Port: 9090, Path: "/sub"})
	var raw []byte
	if err := s.DB.QueryRowContext(ctx, "SELECT body FROM subscription_settings WHERE id=1").Scan(&raw); err == nil {
		var saved Settings
		if err := json.Unmarshal(raw, &saved); err == nil {
			sub.settings = saved
			if err := sub.Apply(ctx, saved, false); err != nil {
				sub.startupError = err.Error()
			}
		}
	}
	go func() {
		<-ctx.Done()
		sub.mu.Lock()
		defer sub.mu.Unlock()
		if sub.server != nil {
			_ = sub.server.Close()
		}
	}()

	return sub
}
func (sub *Service) Status() map[string]any {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	return map[string]any{"settings": sub.settings, "running": sub.server != nil, "error": sub.startupError}
}

func (sub *Service) Close() {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.server != nil {
		_ = sub.server.Close()
		sub.server = nil
	}
}
