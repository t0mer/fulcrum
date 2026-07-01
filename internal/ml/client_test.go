package ml

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetectParsesFaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/detect" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("content-type = %q, want multipart", ct)
		}
		// The uploaded image must arrive under the "file" field.
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer f.Close()
		if b, _ := io.ReadAll(f); string(b) != "IMG" {
			t.Errorf("uploaded body = %q, want IMG", b)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"faces":[{"bbox":[1,2,3,4],"det_score":0.9,"embedding":[0.1,0.2]}]}`)
	}))
	defer srv.Close()

	faces, err := New(srv.URL).Detect(context.Background(), []byte("IMG"), "x.jpg")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(faces) != 1 {
		t.Fatalf("faces = %d, want 1", len(faces))
	}
	if faces[0].DetScore != 0.9 || len(faces[0].Embedding) != 2 {
		t.Errorf("unexpected face %+v", faces[0])
	}
}

func TestDetectErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"model not loaded"}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := New(srv.URL).Detect(context.Background(), []byte("x"), "x.jpg"); err == nil {
		t.Fatal("expected error on 503")
	}
}

func TestReadyzProbe(t *testing.T) {
	var ready bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Readyz(context.Background()); err == nil {
		t.Fatal("expected Readyz error while not ready")
	}
	ready = true
	if err := c.Readyz(context.Background()); err != nil {
		t.Fatalf("Readyz after ready: %v", err)
	}
}
