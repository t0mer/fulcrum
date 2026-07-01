package whatsapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func init() { Register("wwebjs", func(c Config) Provider { return &wwebjs{cfg: c} }) }

// wwebjsSession is the wwebjs-api session id Fulcrum drives. Start the gateway
// session under this id (see CLAUDE.md §7).
const wwebjsSession = "fulcrum"

// wwebjs adapts wwebjs-api (a Chromium-backed gateway; heavy on arm64).
// Auth is an API key sent as x-api-key. Token = api key.
type wwebjs struct{ cfg Config }

func (w *wwebjs) Name() string { return "wwebjs" }

func (w *wwebjs) url(path string) string {
	return fmt.Sprintf("%s/%s/%s", w.cfg.base(), strings.Trim(path, "/"), wwebjsSession)
}

func (w *wwebjs) auth(req *http.Request) {
	if w.cfg.Token != "" {
		req.Header.Set("x-api-key", w.cfg.Token)
	}
}

type wwebjsWebhook struct {
	DataType string `json:"dataType"`
	Data     struct {
		Message struct {
			ID struct {
				Serialized string `json:"_serialized"`
			} `json:"id"`
			From      string `json:"from"`
			HasMedia  bool   `json:"hasMedia"`
			Type      string `json:"type"`
			Body      string `json:"body"`
			Timestamp int64  `json:"timestamp"`
		} `json:"message"`
	} `json:"data"`
}

func (w *wwebjs) ParseWebhook(r *http.Request) ([]InboundMessage, error) {
	var wh wwebjsWebhook
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&wh); err != nil {
		return nil, fmt.Errorf("decoding wwebjs webhook: %w", err)
	}
	if wh.DataType != "message" {
		return nil, nil
	}
	m := wh.Data.Message
	if !strings.Contains(m.From, "@g.us") {
		return nil, nil
	}
	msg := InboundMessage{
		ProviderGroupID: m.From,
		MessageID:       m.ID.Serialized,
		Timestamp:       time.Unix(m.Timestamp, 0),
	}
	if m.HasMedia && m.Type == "image" {
		msg.IsImage = true
		msg.Caption = m.Body
		// Media is fetched lazily via downloadMedia using the message id.
		msg.MediaRef = "wa:" + m.From + "|" + m.ID.Serialized
	}
	return []InboundMessage{msg}, nil
}

func (w *wwebjs) DownloadMedia(ctx context.Context, m InboundMessage) ([]byte, string, error) {
	ref := strings.TrimPrefix(m.MediaRef, "wa:")
	chatID, messageID, ok := strings.Cut(ref, "|")
	if !ok {
		return nil, "", fmt.Errorf("malformed wwebjs media ref")
	}
	payload, _ := json.Marshal(map[string]string{"chatId": chatID, "messageId": messageID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url("message/downloadMedia"), bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	w.auth(req)
	resp, err := w.cfg.httpClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("downloading media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("downloadMedia returned %d", resp.StatusCode)
	}
	var body struct {
		Media struct {
			Mimetype string `json:"mimetype"`
			Data     string `json:"data"`
		} `json:"media"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, "", fmt.Errorf("decoding media: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(body.Media.Data)
	if err != nil {
		return nil, "", fmt.Errorf("decoding media base64: %w", err)
	}
	mime := body.Media.Mimetype
	if mime == "" {
		mime = "image/jpeg"
	}
	return data, mime, nil
}

func (w *wwebjs) ListGroups(ctx context.Context) ([]Group, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.url("client/getChats"), nil)
	if err != nil {
		return nil, err
	}
	w.auth(req)
	resp, err := w.cfg.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing chats: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getChats returned %d", resp.StatusCode)
	}
	var body struct {
		Chats []struct {
			ID struct {
				Serialized string `json:"_serialized"`
			} `json:"id"`
			Name    string `json:"name"`
			IsGroup bool   `json:"isGroup"`
		} `json:"chats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding chats: %w", err)
	}
	var out []Group
	for _, c := range body.Chats {
		if c.IsGroup {
			out = append(out, Group{ProviderGroupID: c.ID.Serialized, Name: c.Name})
		}
	}
	return out, nil
}

func (w *wwebjs) SendImage(ctx context.Context, groupID string, img []byte, mime, caption string) error {
	payload, _ := json.Marshal(map[string]any{
		"chatId":      groupID,
		"contentType": "MessageMedia",
		"content": map[string]string{
			"mimetype": mime,
			"data":     base64.StdEncoding.EncodeToString(img),
			"filename": "match" + extForMime(mime),
		},
		"options": map[string]string{"caption": caption},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url("client/sendMessage"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	w.auth(req)
	resp, err := w.cfg.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("sendMessage returned %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	return nil
}
