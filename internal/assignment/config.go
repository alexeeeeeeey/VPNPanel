package assignment

import (
	"context"
	"database/sql"
	"errors"

	"vpnpanel/internal/config"
	"vpnpanel/internal/store"
)

func Variables(ctx context.Context, s *store.Store) (string, map[string]any, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, "SELECT body FROM panel_variables WHERE id=1").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		raw = "{}"
	} else if err != nil {
		return "", nil, err
	}
	values, err := config.Decode(raw)
	return raw, values, err
}

type State struct {
	TemplateID string   `json:"template_id"`
	Missing    []string `json:"missing"`
	Ready      bool     `json:"ready"`
	Error      string   `json:"error,omitempty"`
}

func ActiveConfig(ctx context.Context, s *store.Store) (State, []byte) {
	state := State{Missing: []string{}}
	err := s.DB.QueryRowContext(ctx, "SELECT template_id FROM node_config WHERE id=1").Scan(&state.TemplateID)
	if errors.Is(err, sql.ErrNoRows) || state.TemplateID == "" {
		return state, nil
	}
	if err != nil {
		state.Error = "Не удалось прочитать назначение"
		return state, nil
	}
	var body, kind string
	if err = s.DB.QueryRowContext(ctx, "SELECT body,kind FROM templates WHERE id=?", state.TemplateID).Scan(&body, &kind); err != nil || kind != "server" {
		state.Error = "Серверный конфиг недоступен"
		return state, nil
	}
	_, values, err := Variables(ctx, s)
	if err != nil {
		state.Error = err.Error()
		return state, nil
	}
	state.Missing, err = config.Missing(body, values)
	if err != nil {
		state.Error = err.Error()
		return state, nil
	}
	if len(state.Missing) > 0 {
		return state, nil
	}
	users, err := s.Users(ctx)
	if err != nil {
		state.Error = err.Error()
		return state, nil
	}
	out, err := config.RenderWithVariables(body, "server", users, nil, values)
	if err != nil {
		state.Error = err.Error()
		return state, nil
	}
	state.Ready = true
	return state, out
}
