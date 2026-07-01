package enroll

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/t0mer/fulcrum/internal/ml"
	"github.com/t0mer/fulcrum/internal/store"
)

// fakeDetector returns a scripted set of faces (optionally per-call).
type fakeDetector struct {
	faces []ml.Face
	err   error
}

func (f *fakeDetector) Detect(_ context.Context, _ []byte, _ string) ([]ml.Face, error) {
	return f.faces, f.err
}

func newTestSvc(t *testing.T, det Detector) (*Service, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	faces := filepath.Join(dir, "faces")
	return New(st, det, faces), st, faces
}

func face(score float32) ml.Face {
	return ml.Face{BBox: []float64{0, 0, 1, 1}, DetScore: float64(score), Embedding: []float32{0.1, 0.2, 0.3}}
}

func TestEnrollSingleFaceSavesFileAndEmbedding(t *testing.T) {
	svc, st, facesRoot := newTestSvc(t, &fakeDetector{faces: []ml.Face{face(0.99)}})
	sub, _ := st.CreateSubject("Yael", "yael", nil)

	res, err := svc.EnrollFace(context.Background(), sub, []byte("img"), ".jpg", -1)
	if err != nil {
		t.Fatalf("EnrollFace: %v", err)
	}
	if res.Face == nil {
		t.Fatal("expected a stored face")
	}
	// File landed under faces/{slug}/.
	if got := filepath.Dir(res.Face.SourcePath); got != filepath.Join(facesRoot, "yael") {
		t.Errorf("stored under %s, want faces/yael", got)
	}
	if _, err := os.Stat(res.Face.SourcePath); err != nil {
		t.Errorf("file not written: %v", err)
	}
	faces, _ := st.ListFaces(sub.ID)
	if len(faces) != 1 {
		t.Errorf("stored faces = %d, want 1", len(faces))
	}
}

func TestEnrollNoFaceRejected(t *testing.T) {
	svc, st, _ := newTestSvc(t, &fakeDetector{faces: nil})
	sub, _ := st.CreateSubject("Noa", "noa", nil)

	_, err := svc.EnrollFace(context.Background(), sub, []byte("img"), ".jpg", -1)
	if !errors.Is(err, ErrNoFace) {
		t.Fatalf("err = %v, want ErrNoFace", err)
	}
}

func TestEnrollMultipleFacesNeedsSelection(t *testing.T) {
	svc, st, _ := newTestSvc(t, &fakeDetector{faces: []ml.Face{face(0.9), face(0.8)}})
	sub, _ := st.CreateSubject("Amit", "amit", nil)

	res, err := svc.EnrollFace(context.Background(), sub, []byte("img"), ".jpg", -1)
	if !errors.Is(err, ErrNeedsSelection) {
		t.Fatalf("err = %v, want ErrNeedsSelection", err)
	}
	if len(res.Candidates) != 2 {
		t.Errorf("candidates = %d, want 2", len(res.Candidates))
	}
	// Nothing should have been stored.
	if faces, _ := st.ListFaces(sub.ID); len(faces) != 0 {
		t.Errorf("stored faces = %d, want 0", len(faces))
	}
}

func TestEnrollMultipleFacesWithIndexStores(t *testing.T) {
	svc, st, _ := newTestSvc(t, &fakeDetector{faces: []ml.Face{face(0.9), face(0.8)}})
	sub, _ := st.CreateSubject("Amit", "amit", nil)

	res, err := svc.EnrollFace(context.Background(), sub, []byte("img"), ".jpg", 1)
	if err != nil {
		t.Fatalf("EnrollFace: %v", err)
	}
	if res.Face == nil {
		t.Fatal("expected a stored face")
	}
}

func TestEnrollRejectsBadExtension(t *testing.T) {
	svc, st, _ := newTestSvc(t, &fakeDetector{faces: []ml.Face{face(0.99)}})
	sub, _ := st.CreateSubject("X", "x", nil)
	if _, err := svc.EnrollFace(context.Background(), sub, []byte("img"), "../etc", -1); err == nil {
		t.Fatal("expected error for bad extension")
	}
}

func TestReembedRewritesFromDisk(t *testing.T) {
	det := &fakeDetector{faces: []ml.Face{face(0.99)}}
	svc, st, _ := newTestSvc(t, det)
	sub, _ := st.CreateSubject("Yael", "yael", nil)

	// Enroll two originals.
	for i := 0; i < 2; i++ {
		if _, err := svc.EnrollFace(context.Background(), sub, []byte("img"), ".jpg", -1); err != nil {
			t.Fatalf("EnrollFace: %v", err)
		}
	}
	// Re-embed should find both files and rewrite embeddings.
	n, err := svc.ReembedSubject(context.Background(), sub)
	if err != nil {
		t.Fatalf("ReembedSubject: %v", err)
	}
	if n != 2 {
		t.Errorf("re-embedded = %d, want 2", n)
	}
}

func TestRemoveSubjectFacesRejectsBadSlug(t *testing.T) {
	svc, _, _ := newTestSvc(t, &fakeDetector{})
	if err := svc.RemoveSubjectFaces("../../etc"); err == nil {
		t.Fatal("expected error for invalid slug")
	}
}
