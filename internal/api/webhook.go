package api

import (
	"crypto/subtle"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// webhook is the inbound gateway callback. It verifies the shared secret,
// normalizes the payload via the active provider, drops anything that isn't an
// image from a monitored group, and enqueues the rest. It returns 200 quickly;
// all processing happens on the worker pool.
func (a *API) webhook(w http.ResponseWriter, r *http.Request) {
	if name := chi.URLParam(r, "provider"); name != "" && name != a.provName {
		writeError(w, http.StatusNotFound, "unknown or inactive provider")
		return
	}
	if !a.verifySecret(r) {
		writeError(w, http.StatusUnauthorized, "invalid webhook secret")
		return
	}
	if a.provider == nil {
		writeError(w, http.StatusServiceUnavailable, "no provider configured")
		return
	}

	messages, err := a.provider.ParseWebhook(r)
	if err != nil {
		a.log.Warn("parse webhook", "err", err)
		// Ack anyway so the gateway doesn't retry a payload we can't parse.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	enqueued := 0
	for _, m := range messages {
		a.incInbound(m.ProviderGroupID)
		if !m.IsImage {
			continue
		}
		monitored, err := a.store.IsMonitored(m.ProviderGroupID)
		if err != nil {
			a.log.Error("check monitored", "err", err)
			continue
		}
		if !monitored {
			continue
		}
		if _, err := a.store.EnqueueJob(a.provName, m.ProviderGroupID, m.MessageID, m.MediaRef); err != nil {
			a.log.Error("enqueue job", "err", err)
			continue
		}
		enqueued++
	}
	if enqueued > 0 && a.notifier != nil {
		a.notifier.Notify()
	}
	writeJSON(w, http.StatusOK, map[string]int{"enqueued": enqueued})
}

// verifySecret checks the X-Webhook-Secret header against the configured value
// in constant time. The secret is only accepted in the header (never the query
// string, which leaks into logs). When no secret is configured the endpoint is
// open (documented bootstrap behaviour, §16).
func (a *API) verifySecret(r *http.Request) bool {
	if a.secret == "" {
		return true
	}
	got := r.Header.Get("X-Webhook-Secret")
	return subtle.ConstantTimeCompare([]byte(got), []byte(a.secret)) == 1
}

func (a *API) incInbound(groupID string) {
	if a.metrics != nil {
		a.metrics.InboundMessages.WithLabelValues(a.provName, a.store.GroupName(groupID)).Inc()
	}
}
