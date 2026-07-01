package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

const (
	refInline = "b64:" // b64:<mime>:<base64>
	refURL    = "url:" // url:<absolute-url>
)

// inlineRef builds a MediaRef that carries the media bytes inline.
func inlineRef(mime string, data []byte) string {
	return refInline + mime + ":" + base64.StdEncoding.EncodeToString(data)
}

// urlRef builds a MediaRef that points at a URL to fetch.
func urlRef(u string) string { return refURL + u }

// errBlockedAddr rejects a connection to a non-public address (SSRF guard).
var errBlockedAddr = fmt.Errorf("refusing to connect to a non-public address")

// resolveMedia turns a MediaRef into bytes. url refs are fetched through an
// SSRF-guarded client: only http/https, redirects disabled, and connections to
// loopback/private/link-local addresses rejected at dial time (which also
// defeats DNS rebinding). The Authorization header is attached only when the
// media host matches authHost (the provider's own host), so a provider-supplied
// URL to a third party never receives our bearer token.
func resolveMedia(ctx context.Context, ref, authHeader, authHost string) ([]byte, string, error) {
	switch {
	case strings.HasPrefix(ref, refInline):
		rest := strings.TrimPrefix(ref, refInline)
		i := strings.IndexByte(rest, ':')
		if i < 0 {
			return nil, "", fmt.Errorf("malformed inline media ref")
		}
		mime := rest[:i]
		data, err := base64.StdEncoding.DecodeString(rest[i+1:])
		if err != nil {
			return nil, "", fmt.Errorf("decoding inline media: %w", err)
		}
		return data, mime, nil

	case strings.HasPrefix(ref, refURL):
		raw := strings.TrimPrefix(ref, refURL)
		u, err := url.Parse(raw)
		if err != nil {
			return nil, "", fmt.Errorf("parsing media url: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, "", fmt.Errorf("media url scheme %q not allowed", u.Scheme)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, "", err
		}
		if authHeader != "" && authHost != "" && strings.EqualFold(u.Hostname(), authHost) {
			req.Header.Set("Authorization", authHeader)
		}

		resp, err := safeMediaClient().Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("fetching media: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("media fetch returned %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return nil, "", err
		}
		mime := resp.Header.Get("Content-Type")
		if mime == "" {
			mime = "image/jpeg"
		}
		return data, mime, nil

	default:
		return nil, "", fmt.Errorf("unrecognized media ref")
	}
}

// safeMediaClient returns an HTTP client that refuses redirects and refuses to
// dial non-public addresses.
func safeMediaClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		// Control runs after DNS resolution with the concrete IP:port, so a
		// hostname resolving to a private IP (incl. DNS-rebinding) is blocked.
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || isBlockedIP(ip) {
				return errBlockedAddr
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{DialContext: dialer.DialContext},
	}
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified()
}

// hostOf returns the hostname portion of a base URL, for auth-host matching.
func hostOf(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
