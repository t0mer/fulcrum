package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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

type reviewReq struct {
	Decision string `json:"decision"` // confirm | reject
}

// reviewMatch confirms or rejects a match. Rejecting deletes the stored file
// (§11): a false positive should not linger on disk.
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
	var status string
	switch req.Decision {
	case "confirm":
		status = "confirmed"
	case "reject":
		status = "rejected"
	default:
		writeError(w, http.StatusBadRequest, "decision must be confirm or reject")
		return
	}

	m, err := a.store.GetMatch(id)
	if err != nil {
		a.respondLookup(w, err)
		return
	}
	if err := a.store.SetReviewed(id, status); err != nil {
		a.respondLookup(w, err)
		return
	}
	if status == "rejected" && m.StoredPath != "" {
		if err := os.Remove(m.StoredPath); err != nil && !os.IsNotExist(err) {
			a.log.Warn("removing rejected match file", "path", m.StoredPath, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"reviewed": status})
}
