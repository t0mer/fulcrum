package whatsapp

import (
	"io"
	"net/http"
)

// newGowaMock returns a handler emulating the gowa endpoints the tests exercise.
func newGowaMock(sentTo *string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/user/my/groups", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"results":{"data":[{"JID":"111@g.us","Name":"Family"}]}}`)
	})
	mux.HandleFunc("/send/image", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		*sentTo = r.FormValue("phone")
		io.WriteString(w, `{"code":"SUCCESS"}`)
	})
	return mux
}
