// Package enroll implements the "teach it my kids' faces" flow: it detects a
// face in an uploaded reference photo, saves the original under
// faces/{slug}/, and stores the embedding. It also re-embeds from the retained
// originals after a model swap. See CLAUDE.md §9.
package enroll

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/google/uuid"

	"github.com/t0mer/fulcrum/internal/ml"
	"github.com/t0mer/fulcrum/internal/store"
)

var (
	// ErrNoFace is returned when detection finds no face in the upload.
	ErrNoFace = errors.New("no face detected in image")
	// ErrNeedsSelection is returned when several faces are found and the
	// caller did not indicate which one to enroll.
	ErrNeedsSelection = errors.New("multiple faces detected; selection required")

	slugRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	extRe  = regexp.MustCompile(`^\.[a-z0-9]{1,5}$`)
)

// Detector is the subset of the ML client the enroll service needs.
type Detector interface {
	Detect(ctx context.Context, image []byte, filename string) ([]ml.Face, error)
}

// Service performs enrollment against the store and ML sidecar.
type Service struct {
	store     *store.Store
	detector  Detector
	facesRoot string
}

// New constructs the enrollment service. facesRoot is the parent of the
// per-subject folders (e.g. /data/faces).
func New(st *store.Store, det Detector, facesRoot string) *Service {
	return &Service{store: st, detector: det, facesRoot: facesRoot}
}

// ValidSlug reports whether a slug matches ^[a-z0-9-]{1,32}$.
func ValidSlug(slug string) bool { return slugRe.MatchString(slug) }

// EnrollResult carries the outcome of an EnrollFace call: either a stored face
// or, when disambiguation is needed, the candidate detections.
type EnrollResult struct {
	Face       *store.Face
	Candidates []ml.Face
}

// EnrollFace detects a face in image, saves the original to faces/{slug}/, and
// stores its embedding. ext is the desired file extension (e.g. ".jpg").
//
// faceIndex selects which detected face to enroll; pass a negative value to
// let the service auto-pick when exactly one face is present. When several
// faces are present and faceIndex is out of range, the candidates are returned
// with ErrNeedsSelection and nothing is saved.
func (s *Service) EnrollFace(ctx context.Context, subject *store.Subject, image []byte, ext string, faceIndex int) (*EnrollResult, error) {
	if !extRe.MatchString(ext) {
		return nil, fmt.Errorf("invalid extension %q", ext)
	}

	faces, err := s.detector.Detect(ctx, image, "upload"+ext)
	if err != nil {
		return nil, fmt.Errorf("detecting faces: %w", err)
	}

	switch {
	case len(faces) == 0:
		return nil, ErrNoFace
	case len(faces) == 1:
		faceIndex = 0
	case faceIndex < 0 || faceIndex >= len(faces):
		return &EnrollResult{Candidates: faces}, ErrNeedsSelection
	}

	dir := s.subjectDir(subject.Slug)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating faces dir: %w", err)
	}
	path := filepath.Join(dir, uuid.NewString()+ext)
	if err := os.WriteFile(path, image, 0o640); err != nil {
		return nil, fmt.Errorf("writing reference photo: %w", err)
	}

	face, err := s.store.AddFace(subject.ID, faces[faceIndex].Embedding, path)
	if err != nil {
		// Roll back the file so DB and disk stay consistent.
		_ = os.Remove(path)
		return nil, err
	}
	return &EnrollResult{Face: face}, nil
}

// ReembedSubject re-runs detection over every retained original in
// faces/{slug}/ and rewrites the subject's embeddings. Files with no detectable
// face are skipped; when several faces are present the highest-scoring one is
// used. Returns the number of embeddings written.
func (s *Service) ReembedSubject(ctx context.Context, subject *store.Subject) (int, error) {
	dir := s.subjectDir(subject.Slug)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, s.store.ReplaceFaces(subject.ID, nil, nil)
		}
		return 0, fmt.Errorf("reading faces dir: %w", err)
	}

	var embeddings [][]float32
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, fmt.Errorf("reading %s: %w", path, err)
		}
		faces, err := s.detector.Detect(ctx, data, e.Name())
		if err != nil {
			return 0, fmt.Errorf("detecting in %s: %w", path, err)
		}
		best := bestFace(faces)
		if best == nil {
			continue // no face in this original; skip
		}
		embeddings = append(embeddings, best.Embedding)
		paths = append(paths, path)
	}

	if err := s.store.ReplaceFaces(subject.ID, embeddings, paths); err != nil {
		return 0, err
	}
	return len(embeddings), nil
}

// ReembedAll re-embeds every subject (e.g. after a model swap).
func (s *Service) ReembedAll(ctx context.Context) (int, error) {
	subjects, err := s.store.ListSubjects()
	if err != nil {
		return 0, err
	}
	total := 0
	for i := range subjects {
		n, err := s.ReembedSubject(ctx, &subjects[i])
		if err != nil {
			return total, fmt.Errorf("re-embedding %s: %w", subjects[i].Slug, err)
		}
		total += n
	}
	return total, nil
}

// RemoveSubjectFaces deletes a subject's entire enrollment folder. The slug is
// re-validated to keep this from touching anything outside facesRoot.
func (s *Service) RemoveSubjectFaces(slug string) error {
	if !ValidSlug(slug) {
		return fmt.Errorf("refusing to remove invalid slug %q", slug)
	}
	return os.RemoveAll(s.subjectDir(slug))
}

func (s *Service) subjectDir(slug string) string {
	return filepath.Join(s.facesRoot, slug)
}

func bestFace(faces []ml.Face) *ml.Face {
	var best *ml.Face
	for i := range faces {
		if best == nil || faces[i].DetScore > best.DetScore {
			best = &faces[i]
		}
	}
	return best
}
