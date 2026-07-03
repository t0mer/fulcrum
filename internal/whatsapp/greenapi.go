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

	chatbot "github.com/green-api/whatsapp-chatbot-golang"
)

func init() { Register("greenapi", func(c Config) Provider { return &greenapi{cfg: c} }) }

// greenapi receives via the bot library's polling loop, not an HTTP webhook.
var _ Receiver = (*greenapi)(nil)

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

// toMessage normalizes a decoded green-api webhook body. ok is false unless the
// payload is an incoming message from a group; a group message that isn't an
// image is still returned (IsImage=false) so intake can count it before
// dropping it. This is the single parse shared by the HTTP webhook route
// (ParseWebhook) and the polling receiver (Receive).
func (wh greenapiWebhook) toMessage() (InboundMessage, bool) {
	if wh.TypeWebhook != "incomingMessageReceived" {
		return InboundMessage{}, false
	}
	groupID := wh.SenderData.ChatID
	if !strings.Contains(groupID, "@g.us") {
		return InboundMessage{}, false
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
	return msg, true
}

// ParseWebhook implements the HTTP-webhook path. In production green-api intake
// runs through Receive (the bot library); this remains a working fallback for
// the /webhook/greenapi route.
func (g *greenapi) ParseWebhook(r *http.Request) ([]InboundMessage, error) {
	var wh greenapiWebhook
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&wh); err != nil {
		return nil, fmt.Errorf("decoding green-api webhook: %w", err)
	}
	msg, ok := wh.toMessage()
	if !ok {
		return nil, nil
	}
	return []InboundMessage{msg}, nil
}

// Receive polls green-api for incoming notifications using the official bot
// library (ReceiveNotification/DeleteNotification), delivering each normalized
// message to handle until ctx is cancelled. This is the default intake path for
// green-api — no inbound HTTP webhook is required. See CLAUDE.md §7.
func (g *greenapi) Receive(ctx context.Context, handle func(InboundMessage)) error {
	// The bot library logs through the std logger with no injectable logger,
	// leaking message bodies and flooding on auth failure; filter it while the
	// receiver runs.
	restoreLog := installBotLogFilter()
	defer restoreLog()

	id, token := g.creds()
	bot := chatbot.NewBot(id, token)
	if g.cfg.BaseURL != "" {
		bot.GreenAPI.APIURL = g.base()
	}
	// Keep notifications queued while Fulcrum is down rather than the library
	// default of flushing them on startup — we must not miss the kids' photos.
	bot.CleanNotificationQueue = false
	bot.IncomingMessageHandler(func(n *chatbot.Notification) {
		if m, ok := fromBody(n.Body); ok {
			handle(m)
		}
	})
	// StartReceivingNotifications blocks; unblock it on shutdown.
	go func() {
		<-ctx.Done()
		bot.StopReceivingNotifications()
	}()
	bot.StartReceivingNotifications()
	return ctx.Err()
}

// fromBody converts the bot library's raw notification body (an already-decoded
// map) into a normalized message by round-tripping it through the same struct
// the webhook path uses.
func fromBody(body map[string]interface{}) (InboundMessage, bool) {
	raw, err := json.Marshal(body)
	if err != nil {
		return InboundMessage{}, false
	}
	var wh greenapiWebhook
	if err := json.Unmarshal(raw, &wh); err != nil {
		return InboundMessage{}, false
	}
	return wh.toMessage()
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
