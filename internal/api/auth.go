package api

import (
	"crypto/subtle"
	"net/http"
)

// authInfo (public) tells the SPA whether the API requires a token, so it can
// decide whether to show a login gate.
func (a *API) authInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"auth_required": a.authToken != ""})
}

// authMiddleware protects the /api surface when an auth token is configured.
// A caller authenticates with either the X-API-Token header or HTTP Basic Auth
// (the token as the password). When no token is set the API is open (bootstrap).
func (a *API) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.authToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		if a.authorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="fulcrum"`)
		writeError(w, http.StatusUnauthorized, "authentication required")
	})
}

func (a *API) authorized(r *http.Request) bool {
	if tok := r.Header.Get("X-API-Token"); tok != "" {
		return tokenEqual(tok, a.authToken)
	}
	if _, pass, ok := r.BasicAuth(); ok {
		return tokenEqual(pass, a.authToken)
	}
	return false
}

func tokenEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
