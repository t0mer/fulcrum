package whatsapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// greenapiBody builds the raw notification body the bot library hands to the
// IncomingMessageHandler (the "body" object of a green-api webhook).
func greenapiBody(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("bad test body: %v", err)
	}
	return m
}

func TestFromBody_ImageInGroup(t *testing.T) {
	body := greenapiBody(t, `{
		"typeWebhook": "incomingMessageReceived",
		"idMessage": "ABC123",
		"timestamp": 1700000000,
		"senderData": {"chatId": "120363000000000000@g.us"},
		"messageData": {
			"typeMessage": "imageMessage",
			"fileMessageData": {
				"downloadUrl": "https://media.green-api.com/pic.jpg",
				"caption": "look",
				"mimeType": "image/jpeg"
			}
		}
	}`)

	m, ok := fromBody(body)
	if !ok {
		t.Fatal("expected ok for an incoming group image")
	}
	if m.ProviderGroupID != "120363000000000000@g.us" {
		t.Errorf("group = %q", m.ProviderGroupID)
	}
	if m.MessageID != "ABC123" {
		t.Errorf("message id = %q", m.MessageID)
	}
	if !m.IsImage {
		t.Error("expected IsImage=true")
	}
	if m.Caption != "look" {
		t.Errorf("caption = %q", m.Caption)
	}
	if !strings.HasSuffix(m.MediaRef, "https://media.green-api.com/pic.jpg") {
		t.Errorf("media ref = %q, want a url ref to the download url", m.MediaRef)
	}
	if m.Timestamp.Unix() != 1700000000 {
		t.Errorf("timestamp = %v", m.Timestamp)
	}
}

func TestFromBody_DropsDirectChat(t *testing.T) {
	body := greenapiBody(t, `{
		"typeWebhook": "incomingMessageReceived",
		"idMessage": "X",
		"senderData": {"chatId": "9725000000@c.us"},
		"messageData": {"typeMessage": "imageMessage",
			"fileMessageData": {"downloadUrl": "https://media.green-api.com/pic.jpg"}}
	}`)

	if _, ok := fromBody(body); ok {
		t.Fatal("1:1 chat (not @g.us) must be dropped")
	}
}

func TestFromBody_TextInGroupCountedNotImage(t *testing.T) {
	// A non-image group message is still admitted (ok=true) so intake can count
	// it in the inbound metric before dropping it; it must not be an image.
	body := greenapiBody(t, `{
		"typeWebhook": "incomingMessageReceived",
		"idMessage": "T1",
		"senderData": {"chatId": "120363000000000000@g.us"},
		"messageData": {"typeMessage": "textMessage",
			"textMessageData": {"textMessage": "hello"}}
	}`)

	m, ok := fromBody(body)
	if !ok {
		t.Fatal("group text message should be admitted (ok=true)")
	}
	if m.IsImage {
		t.Error("text message must not be flagged as an image")
	}
	if m.MediaRef != "" {
		t.Errorf("text message should have no media ref, got %q", m.MediaRef)
	}
}

func TestFromBody_IgnoresNonIncoming(t *testing.T) {
	body := greenapiBody(t, `{
		"typeWebhook": "outgoingMessageStatus",
		"senderData": {"chatId": "120363000000000000@g.us"}
	}`)
	if _, ok := fromBody(body); ok {
		t.Fatal("non-incoming webhook types must be dropped")
	}
}
