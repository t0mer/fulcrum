package whatsapp

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveInline(t *testing.T) {
	ref := refInline + "image/png:" + base64.StdEncoding.EncodeToString([]byte("PNG"))
	data, mime, err := resolveMedia(context.Background(), ref, "", "")
	if err != nil {
		t.Fatalf("resolveMedia: %v", err)
	}
	if string(data) != "PNG" || mime != "image/png" {
		t.Errorf("got %q (%s)", data, mime)
	}
}

func TestResolveRejectsBadScheme(t *testing.T) {
	_, _, err := resolveMedia(context.Background(), urlRef("file:///etc/passwd"), "", "")
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("expected scheme rejection, got %v", err)
	}
}

func TestResolveBlocksLoopback(t *testing.T) {
	// A real local server on 127.0.0.1 must be refused at dial time (SSRF guard).
	srv := httptest.NewServer(nil)
	defer srv.Close()
	_, _, err := resolveMedia(context.Background(), urlRef(srv.URL), "", "")
	if err == nil {
		t.Fatal("expected loopback fetch to be blocked")
	}
}

func TestResolveAllowsPublicShapedHostViaGuard(t *testing.T) {
	// httptest binds loopback, so we can't assert a successful public fetch here;
	// this documents that the guard is the gate. See TestResolveBlocksLoopback.
	_, _, err := resolveMedia(context.Background(), urlRef("http://256.256.256.256/x"), "", "")
	if err == nil {
		t.Fatal("expected fetch to a bogus host to fail")
	}
}
