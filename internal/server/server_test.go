package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/prometheus/client_golang/prometheus"
)

func testHandler(t *testing.T, ready ReadyFunc) http.Handler {
	t.Helper()
	spa := fstest.MapFS{
		"index.html":    {Data: []byte("<html>fulcrum</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	return New(Options{
		Registry: prometheus.NewRegistry(),
		SPA:      spa,
		Ready:    ready,
	})
}

func TestHealthz(t *testing.T) {
	h := testHandler(t, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", rec.Code)
	}
}

func TestReadyzReflectsDependency(t *testing.T) {
	h := testHandler(t, func() error { return errors.New("db down") })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz = %d, want 503 when dependency errors", rec.Code)
	}
}

func TestMetricsExposed(t *testing.T) {
	h := testHandler(t, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics = %d, want 200", rec.Code)
	}
}

func TestSPAFallbackServesIndex(t *testing.T) {
	h := testHandler(t, nil)
	// A client-side route with no matching file should serve index.html.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/subjects/42", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("spa fallback = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>fulcrum</html>" {
		t.Fatalf("spa fallback body = %q, want index.html", body)
	}
}

func TestSPAServesAsset(t *testing.T) {
	h := testHandler(t, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("asset = %d, want 200", rec.Code)
	}
}
