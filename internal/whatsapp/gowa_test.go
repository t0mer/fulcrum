package whatsapp

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryHasProviders(t *testing.T) {
	for _, name := range []string{"gowa", "greenapi", "wwebjs"} {
		if _, err := New(name, Config{}); err != nil {
			t.Errorf("provider %q not registered: %v", name, err)
		}
	}
	if _, err := New("nope", Config{}); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestGowaParseInlineImage(t *testing.T) {
	img := base64.StdEncoding.EncodeToString([]byte("JPEGBYTES"))
	body := `{
		"chat_id": "12345-6789@g.us",
		"message": {"id": "ABC123"},
		"image": {"caption": "hi", "mime_type": "image/jpeg", "base64": "` + img + `"}
	}`
	req := httptest.NewRequest("POST", "/webhook/gowa", strings.NewReader(body))

	g, _ := New("gowa", Config{})
	msgs, err := g.ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	m := msgs[0]
	if !m.IsImage || m.ProviderGroupID != "12345-6789@g.us" || m.MessageID != "ABC123" {
		t.Fatalf("unexpected message %+v", m)
	}

	// The inline ref should resolve back to the original bytes.
	data, mime, err := g.DownloadMedia(context.Background(), m)
	if err != nil {
		t.Fatalf("DownloadMedia: %v", err)
	}
	if string(data) != "JPEGBYTES" || mime != "image/jpeg" {
		t.Errorf("resolved media = %q (%s)", data, mime)
	}
}

func TestGowaIgnoresNonGroup(t *testing.T) {
	body := `{"chat_id": "12345@s.whatsapp.net", "message": {"id": "x"}}`
	req := httptest.NewRequest("POST", "/webhook/gowa", strings.NewReader(body))
	g, _ := New("gowa", Config{})
	msgs, err := g.ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("non-group message should be dropped, got %d", len(msgs))
	}
}

func TestGowaListGroupsAndSend(t *testing.T) {
	var sentTo string
	srv := httptest.NewServer(newGowaMock(&sentTo))
	defer srv.Close()

	g, _ := New("gowa", Config{BaseURL: srv.URL})
	groups, err := g.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].ProviderGroupID != "111@g.us" || groups[0].Name != "Family" {
		t.Fatalf("unexpected groups %+v", groups)
	}

	if err := g.SendImage(context.Background(), "111@g.us", []byte("img"), "image/jpeg", "cap"); err != nil {
		t.Fatalf("SendImage: %v", err)
	}
	if sentTo != "111@g.us" {
		t.Errorf("sent to %q, want 111@g.us", sentTo)
	}
}
