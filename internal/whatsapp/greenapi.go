package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

func init() { Register("greenapi", func(c Config) Provider { return &greenapi{cfg: c} }) }

// greenapi adapts green-api (a WhatsApp cloud gateway). NOTE: green-api routes
// media through a third-party cloud — the design advises against it for
// children's photos (CLAUDE.md §7); it is implemented for completeness.
//
// Token format: "<idInstance>:<apiToken>". BaseURL defaults to
// https://api.green-api.com but may be a cluster URL from the console.
type greenapi struct{ cfg Config }

func (g *greenapi) Name() string { return "greenapi" }

func (g *greenapi) base() string {
	if g.cfg.BaseURL == "" {
		return "https://api.green-api.com"
	}
	return g.cfg.base()
}

func (g *greenapi) creds() (id, token string) {
	id, token, _ = strings.Cut(g.cfg.Token, ":")
	return
}

func (g *greenapi) endpoint(method string) string {
	id, token := g.creds()
	return fmt.Sprintf("%s/waInstance%s/%s/%s", g.base(), id, method, token)
}

type greenapiWebhook struct {
	TypeWebhook string `json:"typeWebhook"`
	IDMessage   string `json:"idMessage"`
	Timestamp   int64  `json:"timestamp"`
	SenderData  struct {
		ChatID string `json:"chatId"`
	} `json:"senderData"`
	MessageData struct {
		TypeMessage     string `json:"typeMessage"`
		FileMessageData struct {
			DownloadURL string `json:"downloadUrl"`
			Caption     string `json:"caption"`
			MimeType    string `json:"mimeType"`
		} `json:"fileMessageData"`
	} `json:"messageData"`
}

func (g *greenapi) ParseWebhook(r *http.Request) ([]InboundMessage, error) {
	var wh greenapiWebhook
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&wh); err != nil {
		return nil, fmt.Errorf("decoding green-api webhook: %w", err)
	}
	if wh.TypeWebhook != "incomingMessageReceived" {
		return nil, nil
	}
	groupID := wh.SenderData.ChatID
	if !strings.Contains(groupID, "@g.us") {
		return nil, nil
	}
	msg := InboundMessage{
		ProviderGroupID: groupID,
		MessageID:       wh.IDMessage,
		Timestamp:       time.Unix(wh.Timestamp, 0),
	}
	if wh.MessageData.TypeMessage == "imageMessage" && wh.MessageData.FileMessageData.DownloadURL != "" {
		msg.IsImage = true
		msg.Caption = wh.MessageData.FileMessageData.Caption
		msg.MediaRef = urlRef(wh.MessageData.FileMessageData.DownloadURL)
	}
	return []InboundMessage{msg}, nil
}

func (g *greenapi) DownloadMedia(ctx context.Context, m InboundMessage) ([]byte, string, error) {
	// green-api download URLs are pre-authorized; no auth header attached.
	return resolveMedia(ctx, m.MediaRef, "", "")
}

func (g *greenapi) ListGroups(ctx context.Context) ([]Group, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.endpoint("getContacts"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.cfg.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing contacts: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getContacts returned %d", resp.StatusCode)
	}
	var contacts []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&contacts); err != nil {
		return nil, fmt.Errorf("decoding contacts: %w", err)
	}
	var out []Group
	for _, c := range contacts {
		if c.Type == "group" || strings.Contains(c.ID, "@g.us") {
			out = append(out, Group{ProviderGroupID: c.ID, Name: c.Name})
		}
	}
	return out, nil
}

func (g *greenapi) SendImage(ctx context.Context, groupID string, img []byte, mime, caption string) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("chatId", groupID)
	_ = mw.WriteField("caption", caption)
	part, err := mw.CreateFormFile("file", "match"+extForMime(mime))
	if err != nil {
		return err
	}
	if _, err := part.Write(img); err != nil {
		return err
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint("sendFileByUpload"), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := g.cfg.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("sending file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("sendFileByUpload returned %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	return nil
}
