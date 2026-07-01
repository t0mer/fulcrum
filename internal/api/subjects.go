package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/t0mer/fulcrum/internal/enroll"
	"github.com/t0mer/fulcrum/internal/ml"
	"github.com/t0mer/fulcrum/internal/store"
)

// maxUploadBytes caps a single reference photo upload.
const maxUploadBytes = 20 << 20 // 20 MiB

var allowedExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

type subjectResponse struct {
	store.Subject
	FaceCount int `json:"face_count"`
}

func (a *API) listSubjects(w http.ResponseWriter, _ *http.Request) {
	subjects, err := a.store.ListSubjects()
	if err != nil {
		a.log.Error("list subjects", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]subjectResponse, 0, len(subjects))
	for i := range subjects {
		faces, err := a.store.ListFaces(subjects[i].ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, subjectResponse{Subject: subjects[i], FaceCount: len(faces)})
	}
	writeJSON(w, http.StatusOK, out)
}

type createSubjectReq struct {
	Name      string   `json:"name"`
	Slug      string   `json:"slug"`
	Threshold *float64 `json:"threshold"`
}

func (a *API) createSubject(w http.ResponseWriter, r *http.Request) {
	var req createSubjectReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.TrimSpace(req.Slug)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !enroll.ValidSlug(req.Slug) {
		writeError(w, http.StatusBadRequest, "slug must match ^[a-z0-9-]{1,32}$")
		return
	}
	if req.Threshold != nil && (*req.Threshold <= 0 || *req.Threshold >= 1) {
		writeError(w, http.StatusBadRequest, "threshold must be in (0,1)")
		return
	}

	sub, err := a.store.CreateSubject(req.Name, req.Slug, req.Threshold)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "name or slug already exists")
			return
		}
		a.log.Error("create subject", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

type subjectDetail struct {
	store.Subject
	Faces []faceResponse `json:"faces"`
}

type faceResponse struct {
	ID         int64  `json:"id"`
	SubjectID  int64  `json:"subject_id"`
	SourcePath string `json:"source_path"`
	AddedAt    string `json:"added_at"`
}

func (a *API) getSubject(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	sub, err := a.store.GetSubject(id)
	if err != nil {
		a.respondLookup(w, err)
		return
	}
	faces, err := a.store.ListFaces(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	detail := subjectDetail{Subject: *sub, Faces: make([]faceResponse, 0, len(faces))}
	for _, f := range faces {
		detail.Faces = append(detail.Faces, faceResponse{
			ID: f.ID, SubjectID: f.SubjectID, SourcePath: f.SourcePath, AddedAt: f.AddedAt,
		})
	}
	writeJSON(w, http.StatusOK, detail)
}

type updateSubjectReq struct {
	Name           *string  `json:"name"`
	Threshold      *float64 `json:"threshold"`
	ClearThreshold bool     `json:"clear_threshold"`
}

func (a *API) updateSubject(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	var req updateSubjectReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		req.Name = &trimmed
	}
	if req.Threshold != nil && (*req.Threshold <= 0 || *req.Threshold >= 1) {
		writeError(w, http.StatusBadRequest, "threshold must be in (0,1)")
		return
	}
	sub, err := a.store.UpdateSubject(id, req.Name, req.Threshold, req.ClearThreshold)
	if err != nil {
		a.respondLookup(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

func (a *API) deleteSubject(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	sub, err := a.store.DeleteSubject(id)
	if err != nil {
		a.respondLookup(w, err)
		return
	}
	// Remove both the enrollment folder and the collected matches (§21 Q6):
	// deleting a kid leaves nothing of theirs on disk.
	if err := a.enroll.RemoveSubjectFaces(sub.Slug); err != nil {
		a.log.Warn("removing faces dir", "slug", sub.Slug, "err", err)
	}
	if err := a.removeMatchDir(sub.Slug); err != nil {
		a.log.Warn("removing matches dir", "slug", sub.Slug, "err", err)
	}
	writeJSON(w, http.StatusOK, sub)
}

func (a *API) uploadFace(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	sub, err := a.store.GetSubject(id)
	if err != nil {
		a.respondLookup(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "upload too large or malformed")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExts[ext] {
		writeError(w, http.StatusUnsupportedMediaType, "file must be .jpg/.jpeg/.png/.webp")
		return
	}
	data := make([]byte, header.Size)
	if _, err := io.ReadFull(file, data); err != nil {
		writeError(w, http.StatusBadRequest, "could not read upload")
		return
	}

	faceIndex := -1
	if v := r.FormValue("face_index"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			faceIndex = n
		}
	}

	result, err := a.enroll.EnrollFace(r.Context(), sub, data, ext, faceIndex)
	switch {
	case errors.Is(err, enroll.ErrNoFace):
		writeError(w, http.StatusUnprocessableEntity, "no face detected")
		return
	case errors.Is(err, enroll.ErrNeedsSelection):
		writeJSON(w, http.StatusMultipleChoices, map[string]any{
			"error":      "multiple faces detected; resubmit with face_index",
			"candidates": bboxes(result.Candidates),
		})
		return
	case err != nil:
		a.log.Error("enroll face", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, faceResponse{
		ID: result.Face.ID, SubjectID: result.Face.SubjectID,
		SourcePath: result.Face.SourcePath,
	})
}

func (a *API) deleteFace(w http.ResponseWriter, r *http.Request) {
	faceID, ok := a.pathID(w, r, "faceID")
	if !ok {
		return
	}
	face, err := a.store.DeleteFace(faceID)
	if err != nil {
		a.respondLookup(w, err)
		return
	}
	if face.SourcePath != "" {
		if err := os.Remove(face.SourcePath); err != nil && !os.IsNotExist(err) {
			a.log.Warn("removing face file", "path", face.SourcePath, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": faceID})
}

// getFaceImage streams a stored reference photo. The path comes from the DB
// (written by the enroll service with a uuid filename), never from the client.
func (a *API) getFaceImage(w http.ResponseWriter, r *http.Request) {
	faceID, ok := a.pathID(w, r, "faceID")
	if !ok {
		return
	}
	face, err := a.store.GetFace(faceID)
	if err != nil {
		a.respondLookup(w, err)
		return
	}
	f, err := os.Open(face.SourcePath)
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
	http.ServeContent(w, r, filepath.Base(face.SourcePath), info.ModTime(), f)
}

func (a *API) reembedSubject(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	sub, err := a.store.GetSubject(id)
	if err != nil {
		a.respondLookup(w, err)
		return
	}
	n, err := a.enroll.ReembedSubject(r.Context(), sub)
	if err != nil {
		a.log.Error("reembed subject", "err", err)
		writeError(w, http.StatusInternalServerError, "re-embed failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"embeddings": n})
}

func (a *API) reembedAll(w http.ResponseWriter, r *http.Request) {
	n, err := a.enroll.ReembedAll(r.Context())
	if err != nil {
		a.log.Error("reembed all", "err", err)
		writeError(w, http.StatusInternalServerError, "re-embed failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"embeddings": n})
}

// --- helpers ---

func (a *API) pathID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func (a *API) respondLookup(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	a.log.Error("lookup", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// removeMatchDir deletes matches/{slug}/. The slug is re-validated so this can
// never escape the matches root.
func (a *API) removeMatchDir(slug string) error {
	if a.matchesPath == "" || !enroll.ValidSlug(slug) {
		return nil
	}
	return os.RemoveAll(filepath.Join(a.matchesPath, slug))
}

type faceCandidate struct {
	Index    int       `json:"index"`
	BBox     []float64 `json:"bbox"`
	DetScore float64   `json:"det_score"`
}

func bboxes(faces []ml.Face) []faceCandidate {
	out := make([]faceCandidate, len(faces))
	for i, f := range faces {
		out[i] = faceCandidate{Index: i, BBox: f.BBox, DetScore: f.DetScore}
	}
	return out
}

func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
	}
	return false
}
