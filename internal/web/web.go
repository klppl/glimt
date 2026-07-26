// Package web wires the chi router, keeping the ingest hot path separate from
// the auth-gated dashboard.
package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/klppl/glimt/internal/auth"
	"github.com/klppl/glimt/internal/dashboard"
	"github.com/klppl/glimt/internal/ingest"
	"github.com/klppl/glimt/internal/tracker"
)

func Router(col *ingest.Collector, trk *tracker.Handler, dash *dashboard.Handlers, a *auth.Auth) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Ingest hot path — first-party, randomized, signature-free.
	r.Get("/s/{token}.js", trk.Serve)
	r.Post("/e/{token}", col.Handle)
	r.Options("/e/{token}", col.Preflight)
	r.Get("/pixel/{token}.gif", col.HandlePixel)
	r.Get("/pixel/{token}", col.HandlePixel)

	// Static assets (CSS, htmx).
	r.Handle("/assets/*", dashboard.AssetsHandler())

	// Public read-only share dashboards.
	r.Get("/p/{token}", dash.Share)

	// Auth.
	r.Get("/login", dash.LoginForm)
	r.Post("/login", dash.Login)
	r.Get("/logout", dash.Logout)

	// Admin dashboard (gated).
	r.Group(func(pr chi.Router) {
		pr.Use(a.Require)
		pr.Get("/", dash.Index)
		pr.Get("/app/realtime", dash.Realtime)
		pr.Get("/app/export", dash.Export)
		pr.Get("/settings", dash.Settings)
		pr.Post("/settings/sites/create", dash.SiteCreate)
		pr.Post("/settings/sites/delete", dash.SiteDelete)
		pr.Post("/settings/sites/regen", dash.SiteRegen)
		pr.Post("/settings/sites/public", dash.SitePublic)
		pr.Post("/settings/users/create", dash.UserCreate)
		pr.Post("/settings/users/delete", dash.UserDelete)
		pr.Post("/app/funnels/create", dash.FunnelCreate)
		pr.Post("/app/funnels/delete", dash.FunnelDelete)
	})

	return r
}
