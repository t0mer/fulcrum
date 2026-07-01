// Package ml is the Go client for the fulcrum-ml sidecar (/detect, /healthz,
// /readyz). See CLAUDE.md §8 for the API contract.
package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Face is a single detected face with its L2-normalized embedding.
type Face struct {
	BBox      []float64 `json:"bbox"`
	DetScore  float64   `json:"det_score"`
	Embedding []float32 `json:"embedding"`
}

// DetectResponse is the /detect payload.
type DetectResponse struct {
	Faces []Face `json:"faces"`
}

// Client talks to a single fulcrum-ml instance.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client for the given base URL (e.g. http://fulcrum-ml:8081).
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Detect uploads an image and returns the detected faces.
func (c *Client) Detect(ctx context.Context, image []byte, filename string) ([]Face, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(image); err != nil {
		return nil, fmt.Errorf("writing image: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/detect", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling /detect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("/detect returned %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}

	var out DetectResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding /detect response: %w", err)
	}
	return out.Faces, nil
}

// Healthz reports whether the sidecar process is up.
func (c *Client) Healthz(ctx context.Context) error { return c.probe(ctx, "/healthz") }

// Readyz reports whether the sidecar has loaded its model.
func (c *Client) Readyz(ctx context.Context) error { return c.probe(ctx, "/readyz") }

func (c *Client) probe(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d", path, resp.StatusCode)
	}
	return nil
}
