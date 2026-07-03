package whatsapp

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestBotLogWriterDropsMessageBodies(t *testing.T) {
	var buf bytes.Buffer
	w := &botLogWriter{out: &buf}
	in := "Webhook received - map[body:map[messageData:map[textMessageData:map[textMessage:secret]]]]\n"

	n, err := w.Write([]byte(in))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(in) {
		t.Errorf("Write returned %d, want %d (must report full length)", n, len(in))
	}
	if buf.Len() != 0 {
		t.Errorf("message body leaked to log: %q", buf.String())
	}
}

func TestBotLogWriterTrimsRawTail(t *testing.T) {
	var buf bytes.Buffer
	w := &botLogWriter{out: &buf}
	in := "Error unmarshaling webhook top level: invalid character '<'. Raw: <html><title>403 Forbidden</title></html>\n"

	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "403") || strings.Contains(got, "<html>") {
		t.Errorf("raw 403 body spilled into log: %q", got)
	}
	if !strings.Contains(got, "Error unmarshaling webhook top level") {
		t.Errorf("useful error text was dropped: %q", got)
	}
}

func TestBotLogWriterThrottlesRepeats(t *testing.T) {
	var buf bytes.Buffer
	now := time.Unix(0, 0)
	w := &botLogWriter{out: &buf, now: func() time.Time { return now }}
	line := "Error: 403 Forbidden (probably instance data is wrong)\n"

	for i := 0; i < 5; i++ {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if got := strings.Count(buf.String(), "403 Forbidden"); got != 1 {
		t.Errorf("throttle failed: emitted %d times within window, want 1", got)
	}

	// Past the window the same line is allowed through again.
	now = now.Add(botLogRepeatWindow + time.Second)
	if _, err := w.Write([]byte(line)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := strings.Count(buf.String(), "403 Forbidden"); got != 2 {
		t.Errorf("line should re-emit after the window: got %d, want 2", got)
	}
}

func TestBotLogWriterPassesDistinctLines(t *testing.T) {
	var buf bytes.Buffer
	now := time.Unix(0, 0)
	w := &botLogWriter{out: &buf, now: func() time.Time { return now }}

	if _, err := w.Write([]byte("Bot Start receive webhooks\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := w.Write([]byte("Bot stopped\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "Bot Start receive webhooks") || !strings.Contains(got, "Bot stopped") {
		t.Errorf("distinct lifecycle lines were dropped: %q", got)
	}
}
