package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/t0mer/fulcrum/internal/store"
)

func newSettingsAPI(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(Deps{Store: st, ProviderName: "gowa", DefaultThreshold: 0.48, DefaultSinkMode: "both"}).Routes()
}

func TestSettingsReturnsConfigDefaults(t *testing.T) {
	h := newSettingsAPI(t)
	rec := do(t, h, http.MethodGet, "/settings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d, want 200", rec.Code)
	}
	var v settingsView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.GlobalThreshold != 0.48 || v.SinkMode != "both" || v.Provider != "gowa" {
		t.Fatalf("defaults not surfaced: %+v", v)
	}
}

func TestSettingsUpdatePersists(t *testing.T) {
	h := newSettingsAPI(t)
	rec := do(t, h, http.MethodPut, "/settings", `{"global_threshold":0.55,"sink_mode":"storage-only"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d (%s), want 200", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodGet, "/settings", "")
	var v settingsView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.GlobalThreshold != 0.55 || v.SinkMode != "storage-only" {
		t.Fatalf("update not persisted: %+v", v)
	}
}

func TestSettingsRejectsBadValues(t *testing.T) {
	h := newSettingsAPI(t)
	if rec := do(t, h, http.MethodPut, "/settings", `{"global_threshold":2}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad threshold = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodPut, "/settings", `{"sink_mode":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad sink_mode = %d, want 400", rec.Code)
	}
}
