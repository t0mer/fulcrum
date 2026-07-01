package api

import "net/http"

// getProvider reports the active provider and whether it can reach the gateway.
func (a *API) getProvider(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":      a.provName,
		"connected": a.provider != nil,
	})
}

// testProvider checks connectivity by listing groups.
func (a *API) testProvider(w http.ResponseWriter, r *http.Request) {
	if a.provider == nil {
		writeError(w, http.StatusServiceUnavailable, "no provider configured")
		return
	}
	groups, err := a.provider.ListGroups(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "groups": len(groups)})
}
