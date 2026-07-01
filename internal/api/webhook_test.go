package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/fulcrum/internal/store"
	"github.com/t0mer/fulcrum/internal/whatsapp"
)

type fakeNotifier struct{ n int }

func (f *fakeNotifier) Notify() { f.n++ }

func gowaBody(group string) string {
	img := base64.StdEncoding.EncodeToString([]byte("JPEG"))
	return `{"chat_id":"` + group + `","message":{"id":"M1"},` +
		`"image":{"mime_type":"image/jpeg","base64":"` + img + `"}}`
}

func newWebhookAPI(t *testing.T, secret string) (*API, *store.Store, *fakeNotifier) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	prov, _ := whatsapp.New("gowa", whatsapp.Config{})
	notif := &fakeNotifier{}
	a := New(Deps{Store: st, Provider: prov, ProviderName: "gowa", Notifier: notif, WebhookSecret: secret})
	return a, st, notif
}

func postWebhook(a *API, provider, secret, body string) *httptest.ResponseRecorder {
	// Route through chi so the {provider} URL param is populated.
	r := chi.NewRouter()
	r.Post("/webhook/{provider}", a.webhook)
	req := httptest.NewRequest(http.MethodPost, "/webhook/"+provider, strings.NewReader(body))
	if secret != "" {
		req.Header.Set("X-Webhook-Secret", secret)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestWebhookEnqueuesMonitoredGroup(t *testing.T) {
	a, st, notif := newWebhookAPI(t, "")
	_ = st.UpsertGroup("g1@g.us", "Family")
	groups, _ := st.ListGroups()
	_ = st.SetMonitored(groups[0].ID, true)

	rec := postWebhook(a, "gowa", "", gowaBody("g1@g.us"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if pending, _ := st.PendingJobs(); pending != 1 {
		t.Errorf("pending jobs = %d, want 1", pending)
	}
	if notif.n != 1 {
		t.Errorf("notifier calls = %d, want 1", notif.n)
	}
}

func TestWebhookDropsUnmonitoredGroup(t *testing.T) {
	a, st, _ := newWebhookAPI(t, "")
	_ = st.UpsertGroup("g1@g.us", "Family") // not monitored

	rec := postWebhook(a, "gowa", "", gowaBody("g1@g.us"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if pending, _ := st.PendingJobs(); pending != 0 {
		t.Errorf("pending jobs = %d, want 0 (unmonitored)", pending)
	}
}

func TestWebhookRejectsBadSecret(t *testing.T) {
	a, st, _ := newWebhookAPI(t, "s3cr3t")
	_ = st.UpsertGroup("g1@g.us", "Family")
	groups, _ := st.ListGroups()
	_ = st.SetMonitored(groups[0].ID, true)

	rec := postWebhook(a, "gowa", "wrong", gowaBody("g1@g.us"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if pending, _ := st.PendingJobs(); pending != 0 {
		t.Errorf("pending jobs = %d, want 0 (rejected)", pending)
	}
}

func TestWebhookAcceptsGoodSecret(t *testing.T) {
	a, st, _ := newWebhookAPI(t, "s3cr3t")
	_ = st.UpsertGroup("g1@g.us", "Family")
	groups, _ := st.ListGroups()
	_ = st.SetMonitored(groups[0].ID, true)

	rec := postWebhook(a, "gowa", "s3cr3t", gowaBody("g1@g.us"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if pending, _ := st.PendingJobs(); pending != 1 {
		t.Errorf("pending jobs = %d, want 1", pending)
	}
}
