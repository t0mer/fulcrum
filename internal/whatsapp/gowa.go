package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func init() { Register("gowa", func(c Config) Provider { return &gowa{cfg: c} }) }

// gowa adapts go-whatsapp-web-multidevice (a native-Go gateway, no third party
// touches the images). Endpoint/field mappings follow that project's REST API;
// confirm them against your deployed version (see CLAUDE.md §7).
type gowa struct{ cfg Config }

func (g *gowa) Name() string { return "gowa" }

// gowaWebhook models the inbound JSON. encoding/json matches field names
// case-insensitively, so this tolerates JID/jid, Name/name, etc.
type gowaWebhook struct {
	ChatID    string `json:"chat_id"`
	From      string `json:"from"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"message"`
	Image *struct {
		Caption  string `json:"caption"`
		MimeType string `json:"mime_type"`
		Base64   string `json:"base64"` // present when base64 forwarding is on
		URL      string `json:"url"`    // else a fetchable media URL
	} `json:"image"`
}

func (g *gowa) ParseWebhook(r *http.Request) ([]InboundMessage, error) {
	var wh gowaWebhook
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&wh); err != nil {
		return nil, fmt.Errorf("decoding gowa webhook: %w", err)
	}
	groupID := wh.ChatID
	if groupID == "" || !strings.Contains(groupID, "@g.us") {
		// Not a group message; nothing to monitor.
		return nil, nil
	}

	msg := InboundMessage{
		ProviderGroupID: groupID,
		MessageID:       wh.Message.ID,
		Timestamp:       parseGowaTime(wh.Timestamp),
	}
	if wh.Image != nil {
		msg.IsImage = true
		msg.Caption = wh.Image.Caption
		mime := wh.Image.MimeType
		if mime == "" {
			mime = "image/jpeg"
		}
		switch {
		case wh.Image.Base64 != "":
			msg.MediaRef = refInline + mime + ":" + wh.Image.Base64
		case wh.Image.URL != "":
			msg.MediaRef = urlRef(wh.Image.URL)
		}
	}
	return []InboundMessage{msg}, nil
}

func (g *gowa) DownloadMedia(ctx context.Context, m InboundMessage) ([]byte, string, error) {
	return resolveMedia(ctx, m.MediaRef, g.authHeader(), hostOf(g.cfg.base()))
}

func (g *gowa) ListGroups(ctx context.Context) ([]Group, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.cfg.base()+"/user/my/groups", nil)
	if err != nil {
		return nil, err
	}
	g.auth(req)
	resp, err := g.cfg.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("group list returned %d", resp.StatusCode)
	}
	var body struct {
		Results struct {
			Data []struct {
				JID  string `json:"jid"`
				Name string `json:"name"`
			} `json:"data"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding group list: %w", err)
	}
	out := make([]Group, 0, len(body.Results.Data))
	for _, d := range body.Results.Data {
		out = append(out, Group{ProviderGroupID: d.JID, Name: d.Name})
	}
	return out, nil
}

func (g *gowa) SendImage(ctx context.Context, groupID string, img []byte, mime, caption string) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("phone", groupID)
	_ = mw.WriteField("caption", caption)
	part, err := mw.CreateFormFile("image", "match"+extForMime(mime))
	if err != nil {
		return err
	}
	if _, err := part.Write(img); err != nil {
		return err
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.base()+"/send/image", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	g.auth(req)
	resp, err := g.cfg.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("sending image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("send image returned %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	return nil
}

// auth applies basic auth when Token is "user:pass", else a bearer token.
func (g *gowa) auth(req *http.Request) {
	if g.cfg.Token == "" {
		return
	}
	if u, p, ok := strings.Cut(g.cfg.Token, ":"); ok {
		req.SetBasicAuth(u, p)
		return
	}
	req.Header.Set("Authorization", "Bearer "+g.cfg.Token)
}

func (g *gowa) authHeader() string {
	if g.cfg.Token == "" {
		return ""
	}
	if !strings.Contains(g.cfg.Token, ":") {
		return "Bearer " + g.cfg.Token
	}
	return "" // basic auth is applied per-request; media URLs are usually same-origin
}

func parseGowaTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(unix, 0)
	}
	return time.Time{}
}

func extForMime(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}
