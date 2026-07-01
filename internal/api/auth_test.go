package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/t0mer/fulcrum/internal/store"
)

func authAPI(t *testing.T, token string) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(Deps{Store: st, ProviderName: "gowa", AuthToken: token, DefaultSinkMode: "both"}).Routes()
}

func req(t *testing.T, h http.Handler, method, path string, hdr map[string]string) int {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

func TestAuthOpenWhenNoToken(t *testing.T) {
	h := authAPI(t, "")
	if code := req(t, h, http.MethodGet, "/subjects/", nil); code != http.StatusOK {
		t.Errorf("open API = %d, want 200", code)
	}
}

func TestAuthInfoAlwaysPublic(t *testing.T) {
	h := authAPI(t, "s3cr3t")
	if code := req(t, h, http.MethodGet, "/authinfo", nil); code != http.StatusOK {
		t.Errorf("/authinfo = %d, want 200 (public)", code)
	}
}

func TestAuthRejectsMissingToken(t *testing.T) {
	h := authAPI(t, "s3cr3t")
	if code := req(t, h, http.MethodGet, "/subjects/", nil); code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", code)
	}
}

func TestAuthAcceptsHeaderToken(t *testing.T) {
	h := authAPI(t, "s3cr3t")
	code := req(t, h, http.MethodGet, "/subjects/", map[string]string{"X-API-Token": "s3cr3t"})
	if code != http.StatusOK {
		t.Errorf("valid header token = %d, want 200", code)
	}
}

func TestAuthAcceptsBasicAuth(t *testing.T) {
	h := authAPI(t, "s3cr3t")
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:s3cr3t"))
	code := req(t, h, http.MethodGet, "/subjects/", map[string]string{"Authorization": basic})
	if code != http.StatusOK {
		t.Errorf("valid basic auth = %d, want 200", code)
	}
}

func TestAuthRejectsWrongToken(t *testing.T) {
	h := authAPI(t, "s3cr3t")
	code := req(t, h, http.MethodGet, "/subjects/", map[string]string{"X-API-Token": "nope"})
	if code != http.StatusUnauthorized {
		t.Errorf("wrong token = %d, want 401", code)
	}
}
