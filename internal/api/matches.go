package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/t0mer/fulcrum/internal/match"
	"github.com/t0mer/fulcrum/internal/store"
)

func (a *API) listMatches(w http.ResponseWriter, r *http.Request) {
	f := store.MatchFilter{Reviewed: r.URL.Query().Get("reviewed")}
	if v := r.URL.Query().Get("subject_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.SubjectID = id
		}
	}
	matches, err := a.store.ListMatches(f)
	if err != nil {
		a.log.Error("list matches", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if matches == nil {
		matches = []store.Match{}
	}
	writeJSON(w, http.StatusOK, matches)
}

// getMatchImage streams a stored match image. The path is from the DB (written
// by the fs sink), never client-supplied.
func (a *API) getMatchImage(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	m, err := a.store.GetMatch(id)
	if err != nil {
		a.respondLookup(w, err)
		return
	}
	if m.StoredPath == "" {
		writeError(w, http.StatusNotFound, "image was not stored")
		return
	}
	f, err := os.Open(m.StoredPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "image file missing")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, filepath.Base(m.StoredPath), info.ModTime(), f)
}

// tuning returns a subject's review history and a suggested threshold.
func (a *API) tuning(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := a.store.GetSubject(id); err != nil {
		a.respondLookup(w, err)
		return
	}
	stats, err := a.store.TuningStatsFor(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	suggestion := match.SuggestThreshold(stats.MinConfirmed, stats.MaxRejected, stats.ConfirmedCount, stats.RejectedCount)
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats, "suggestion": suggestion})
}

type reviewReq struct {
	Decision string `json:"decision"` // confirm | reject
	// Reinforce (confirm only) adds the matched face to the subject's
	// references. Defaults to true; pass false to just confirm.
	Reinforce *bool `json:"reinforce"`
}

// reviewMatch confirms or rejects a match (active learning, §11). Confirming
// optionally reinforces the subject's references with the matched face;
// rejecting deletes the stored file and records the embedding as a
// hard-negative for threshold tuning.
func (a *API) reviewMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	var req reviewReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Decision != "confirm" && req.Decision != "reject" {
		writeError(w, http.StatusBadRequest, "decision must be confirm or reject")
		return
	}

	m, err := a.store.GetMatch(id)
	if err != nil {
		a.respondLookup(w, err)
		return
	}

	if req.Decision == "reject" {
		if err := a.store.SetReviewed(id, "rejected"); err != nil {
			a.respondLookup(w, err)
			return
		}
		if err := a.store.RecordHardNegative(m.SubjectID, m.Embedding, m.Similarity); err != nil {
			a.log.Warn("record hard negative", "err", err)
		}
		if m.StoredPath != "" {
			if err := os.Remove(m.StoredPath); err != nil && !os.IsNotExist(err) {
				a.log.Warn("removing rejected match file", "path", m.StoredPath, "err", err)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"reviewed": "rejected"})
		return
	}

	// confirm
	if err := a.store.SetReviewed(id, "confirmed"); err != nil {
		a.respondLookup(w, err)
		return
	}
	reinforced := false
	wantReinforce := req.Reinforce == nil || *req.Reinforce
	if wantReinforce && a.enroll != nil && m.StoredPath != "" && len(m.Embedding) > 0 {
		sub, err := a.store.GetSubject(m.SubjectID)
		if err == nil {
			if _, err := a.enroll.Reinforce(sub, m.Embedding, m.StoredPath); err != nil {
				a.log.Warn("reinforce from match", "err", err)
			} else {
				reinforced = true
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviewed": "confirmed", "reinforced": reinforced})
}
