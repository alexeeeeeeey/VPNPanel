package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"vpnpanel/internal/httpx"
	"vpnpanel/internal/store"
	"vpnpanel/internal/subscription"
	"vpnpanel/internal/update"
	"vpnpanel/internal/xray"
)

type Dependencies struct {
	Store         *store.Store
	Subscriptions *subscription.Service
	Sample        func() map[string]any
	Updater       *update.Manager
	Context       context.Context
	Runtime       *xray.Runtime
	Collector     *xray.Collector
}

func New(deps Dependencies) *chi.Mux {
	s := deps.Store
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// No cross-origin mutations: Vite forwards same-origin requests during development.
			if r.Method != "GET" && r.Method != "HEAD" {
				if r.Header.Get("X-Panel-Request") != "1" {
					httpx.Fail(w, 403, "Отсутствует заголовок запроса панели")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	r.Route("/api", func(r chi.Router) {
		mountNodes(r, s.DB)
		mountVariables(r, s)
		mountReplication(r, s)
		mountAssignment(r, s)
		mountTemplateOrder(r, s)
		mountActivity(r, s)
		mountSubscriptions(r, deps.Subscriptions)
		r.Get("/status", func(w http.ResponseWriter, req *http.Request) { httpx.Write(w, 200, deps.Sample()) })
		r.Get("/runtime", func(w http.ResponseWriter, req *http.Request) { httpx.Write(w, 200, deps.Runtime.Status()) })
		r.Get("/usage-status", func(w http.ResponseWriter, req *http.Request) { httpx.Write(w, 200, deps.Collector.Status()) })
		mountUpdates(deps.Context, r, deps.Updater)
		mountUsers(r, s)
		mountTemplates(r, s)
	})
	return r
}
