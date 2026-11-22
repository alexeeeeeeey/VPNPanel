package httpapi

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http"
	"vpnpanel/internal/httpx"
	"vpnpanel/internal/update"
)

func mountUpdates(ctx context.Context, r chi.Router, updater *update.Manager) {
	r.Get("/update", func(w http.ResponseWriter, req *http.Request) {
		state, err := updater.Check(req.Context())
		if err != nil {
			httpx.Fail(w, http.StatusBadGateway, err.Error())
			return
		}
		httpx.Write(w, http.StatusOK, state)
	})
	r.Get("/update-status", func(w http.ResponseWriter, req *http.Request) { httpx.Write(w, http.StatusOK, updater.Status()) })
	r.Post("/update", func(w http.ResponseWriter, req *http.Request) {
		if err := updater.Start(ctx); err != nil {
			httpx.Fail(w, http.StatusConflict, err.Error())
			return
		}
		httpx.Write(w, http.StatusAccepted, updater.Status())
	})
}
