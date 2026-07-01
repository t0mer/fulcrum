package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t0mer/fulcrum/internal/enroll"
	"github.com/t0mer/fulcrum/internal/ml"
	"github.com/t0mer/fulcrum/internal/store"
)

type fakeDetector struct{ faces []ml.Face }

func (f *fakeDetector) Detect(_ context.Context, _ []byte, _ string) ([]ml.Face, error) {
	return f.faces, nil
}

func newTestAPI(t *testing.T, det enroll.Detector) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc := enroll.New(st, det, filepath.Join(t.TempDir(), "faces"))
	return New(st, svc, nil).Routes()
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateAndListSubject(t *testing.T) {
	h := newTestAPI(t, &fakeDetector{})

	rec := do(t, h, http.MethodPost, "/subjects/", `{"name":"יעל","slug":"yael"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d (%s), want 201", rec.Code, rec.Body)
	}

	rec = do(t, h, http.MethodGet, "/subjects/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d, want 200", rec.Code)
	}
	var subs []subjectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &subs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(subs) != 1 || subs[0].Slug != "yael" || subs[0].Name != "יעל" {
		t.Fatalf("unexpected list: %+v", subs)
	}
}

func TestCreateRejectsBadSlug(t *testing.T) {
	h := newTestAPI(t, &fakeDetector{})
	rec := do(t, h, http.MethodPost, "/subjects/", `{"name":"X","slug":"Bad Slug!"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create = %d, want 400", rec.Code)
	}
}

func TestCreateRejectsDuplicateSlug(t *testing.T) {
	h := newTestAPI(t, &fakeDetector{})
	do(t, h, http.MethodPost, "/subjects/", `{"name":"A","slug":"dup"}`)
	rec := do(t, h, http.MethodPost, "/subjects/", `{"name":"B","slug":"dup"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", rec.Code)
	}
}

func TestUploadFaceMultiChoice(t *testing.T) {
	h := newTestAPI(t, &fakeDetector{faces: []ml.Face{
		{BBox: []float64{0, 0, 1, 1}, DetScore: 0.9, Embedding: []float32{0.1}},
		{BBox: []float64{1, 1, 2, 2}, DetScore: 0.8, Embedding: []float32{0.2}},
	}})
	do(t, h, http.MethodPost, "/subjects/", `{"name":"A","slug":"a"}`)

	rec := uploadFile(t, h, "/subjects/1/faces", "photo.jpg", []byte("img"))
	if rec.Code != http.StatusMultipleChoices {
		t.Fatalf("upload = %d, want 300 (needs selection)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "candidates") {
		t.Errorf("expected candidates in body: %s", rec.Body)
	}
}

func TestUploadFaceSingleSucceeds(t *testing.T) {
	h := newTestAPI(t, &fakeDetector{faces: []ml.Face{
		{BBox: []float64{0, 0, 1, 1}, DetScore: 0.99, Embedding: []float32{0.1, 0.2}},
	}})
	do(t, h, http.MethodPost, "/subjects/", `{"name":"A","slug":"a"}`)

	rec := uploadFile(t, h, "/subjects/1/faces", "photo.jpg", []byte("img"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d (%s), want 201", rec.Code, rec.Body)
	}
}

func TestUploadFaceRejectsBadType(t *testing.T) {
	h := newTestAPI(t, &fakeDetector{})
	do(t, h, http.MethodPost, "/subjects/", `{"name":"A","slug":"a"}`)
	rec := uploadFile(t, h, "/subjects/1/faces", "doc.pdf", []byte("img"))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("upload = %d, want 415", rec.Code)
	}
}

func TestDeleteMissingSubject(t *testing.T) {
	h := newTestAPI(t, &fakeDetector{})
	rec := do(t, h, http.MethodDelete, "/subjects/999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete = %d, want 404", rec.Code)
	}
}

func uploadFile(t *testing.T, h http.Handler, path, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", filename)
	part.Write(content)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
