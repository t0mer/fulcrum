// Package api implements the chi REST handlers backing the SPA and the inbound
// webhook intake.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/fulcrum/internal/enroll"
	"github.com/t0mer/fulcrum/internal/metrics"
	"github.com/t0mer/fulcrum/internal/store"
	"github.com/t0mer/fulcrum/internal/whatsapp"
)

// Notifier is woken when new work is enqueued.
type Notifier interface{ Notify() }

// Deps are the API's collaborators.
type Deps struct {
	Store         *store.Store
	Enroll        *enroll.Service
	Provider      whatsapp.Provider
	ProviderName  string
	Notifier      Notifier
	WebhookSecret string
	Metrics       *metrics.Metrics
	Logger        *slog.Logger
	// Config fallbacks surfaced by the settings API when no override is stored.
	DefaultThreshold float64
	DefaultSinkMode  string
	// MatchesPath is the root of the match-output tree, so deleting a subject
	// can also remove its collected images.
	MatchesPath string
}

// API holds handler dependencies.
type API struct {
	store            *store.Store
	enroll           *enroll.Service
	provider         whatsapp.Provider
	provName         string
	notifier         Notifier
	secret           string
	metrics          *metrics.Metrics
	defaultThreshold float64
	defaultSinkMode  string
	matchesPath      string
	log              *slog.Logger
}

// New constructs the API handler set.
func New(d Deps) *API {
	log := d.Logger
	if log == nil {
		log = slog.Default()
	}
	sinkMode := d.DefaultSinkMode
	if sinkMode == "" {
		sinkMode = "both"
	}
	return &API{
		store: d.Store, enroll: d.Enroll, provider: d.Provider,
		provName: d.ProviderName, notifier: d.Notifier, secret: d.WebhookSecret,
		metrics: d.Metrics, defaultThreshold: d.DefaultThreshold, defaultSinkMode: sinkMode,
		matchesPath: d.MatchesPath, log: log,
	}
}

// Routes returns the router to mount under /api.
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Route("/subjects", func(r chi.Router) {
		r.Get("/", a.listSubjects)
		r.Post("/", a.createSubject)
		r.Post("/reembed", a.reembedAll)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", a.getSubject)
			r.Patch("/", a.updateSubject)
			r.Delete("/", a.deleteSubject)
			r.Post("/faces", a.uploadFace)
			r.Get("/faces/{faceID}/image", a.getFaceImage)
			r.Delete("/faces/{faceID}", a.deleteFace)
			r.Post("/reembed", a.reembedSubject)
		})
	})
	r.Route("/groups", func(r chi.Router) {
		r.Get("/", a.listGroups)
		r.Patch("/{id}", a.updateGroup)
	})
	r.Route("/matches", func(r chi.Router) {
		r.Get("/", a.listMatches)
		r.Get("/{id}/image", a.getMatchImage)
		r.Post("/{id}/review", a.reviewMatch)
	})
	r.Get("/provider", a.getProvider)
	r.Post("/provider/test", a.testProvider)
	r.Get("/settings", a.getSettings)
	r.Put("/settings", a.updateSettings)
	r.Get("/docs", a.docs)
	r.Get("/openapi.yaml", a.openapiSpec)
	return r
}

// WebhookHandler handles POST /webhook/{provider} at the server root.
func (a *API) WebhookHandler() http.Handler {
	return http.HandlerFunc(a.webhook)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// decodeJSON decodes a small JSON request body, rejecting unknown fields.
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON body")
	}
	return nil
}
