package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestDocsEndpoints(t *testing.T) {
	h := newSettingsAPI(t) // any wired API is fine; docs need no deps

	rec := do(t, h, http.MethodGet, "/docs", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "swagger-ui") {
		t.Fatalf("/docs = %d, want Swagger UI", rec.Code)
	}

	rec = do(t, h, http.MethodGet, "/openapi.yaml", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/openapi.yaml = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "openapi: 3") {
		t.Errorf("spec does not look like OpenAPI 3")
	}
}
