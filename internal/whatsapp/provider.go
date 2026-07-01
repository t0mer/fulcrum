// Package whatsapp abstracts the WhatsApp gateway behind a single Provider
// interface. All supported gateways integrate over webhook (inbound) + REST
// (outbound); the rest of the pipeline is provider-agnostic. See CLAUDE.md §7.
package whatsapp

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Group is a WhatsApp group as seen by a provider.
type Group struct {
	ProviderGroupID string
	Name            string
}

// InboundMessage is a normalized inbound event.
type InboundMessage struct {
	ProviderGroupID string
	MessageID       string
	IsImage         bool
	// MediaRef is a provider-specific handle the same provider can resolve to
	// bytes via DownloadMedia. Convention: "b64:<mime>:<data>" for inline media
	// or "url:<url>" to fetch.
	MediaRef  string
	Caption   string
	Timestamp time.Time
}

// Provider is the gateway abstraction.
type Provider interface {
	Name() string
	// ParseWebhook turns a raw inbound webhook into normalized messages.
	ParseWebhook(r *http.Request) ([]InboundMessage, error)
	// DownloadMedia resolves a message's media to bytes + mimetype.
	DownloadMedia(ctx context.Context, m InboundMessage) (data []byte, mime string, err error)
	// ListGroups enumerates joined groups (populates the selection UI).
	ListGroups(ctx context.Context) ([]Group, error)
	// SendImage sends an image to a group (used by the forward sink).
	SendImage(ctx context.Context, groupID string, img []byte, mime, caption string) error
}

// Config carries the shared connection settings for a provider.
type Config struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func (c Config) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (c Config) base() string { return strings.TrimRight(c.BaseURL, "/") }

// Factory builds a provider from config.
type Factory func(Config) Provider

var registry = map[string]Factory{}

// Register makes a provider available by name (called from init()).
func Register(name string, f Factory) { registry[name] = f }

// New constructs the named provider.
func New(name string, cfg Config) (Provider, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (have: %s)", name, strings.Join(Names(), ", "))
	}
	return f(cfg), nil
}

// Names lists registered providers.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
