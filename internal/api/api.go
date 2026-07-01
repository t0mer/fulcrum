// Package api implements the chi REST handlers backing the SPA.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/fulcrum/internal/enroll"
	"github.com/t0mer/fulcrum/internal/store"
)

// API holds handler dependencies.
type API struct {
	store  *store.Store
	enroll *enroll.Service
	log    *slog.Logger
}

// New constructs the API handler set.
func New(st *store.Store, en *enroll.Service, log *slog.Logger) *API {
	if log == nil {
		log = slog.Default()
	}
	return &API{store: st, enroll: en, log: log}
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
	return r
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
