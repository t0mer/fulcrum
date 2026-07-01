// Package server wires the chi router: middleware, health/readiness probes,
// the Prometheus endpoint, and the embedded SPA.
package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/t0mer/fulcrum/internal/version"
)

// ReadyFunc reports whether a dependency is ready. Returning an error makes
// /readyz respond 503 with the reason.
type ReadyFunc func() error

// Options configures the router.
type Options struct {
	Logger   *slog.Logger
	Registry *prometheus.Registry
	// SPA is the embedded frontend filesystem (rooted at the built assets).
	SPA fs.FS
	// Ready is consulted by /readyz; nil means always ready.
	Ready ReadyFunc
}

// New builds the top-level HTTP handler.
func New(opts Options) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": version.Version,
		})
	})

	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if opts.Ready != nil {
			if err := opts.Ready(); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"status": "not ready",
					"reason": err.Error(),
				})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	if opts.Registry != nil {
		r.Handle("/metrics", promhttp.HandlerFor(opts.Registry, promhttp.HandlerOpts{}))
	}

	// TODO(milestone-4+): /webhook/{provider} and /api/* mount here.

	if opts.SPA != nil {
		r.NotFound(spaHandler(opts.SPA))
	}

	return r
}

// spaHandler serves static assets and falls back to index.html for client-side
// routes (any path without a file extension that isn't an asset).
func spaHandler(spa fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(spa))
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/" {
			if _, err := fs.Stat(spa, path[1:]); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Client-side route: serve the SPA entrypoint.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
