// Package sink delivers matched images to their destinations: the local
// filesystem and/or a WhatsApp forward. See CLAUDE.md §12.
package sink

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// FS writes matched images under root/{slug}/{YYYY}/{MM}/{messageID}.{ext}.
type FS struct {
	Root string
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// Save stores the image and returns its path. The subject slug is trusted
// (validated at enrollment); the message id is sanitized for use as a filename.
func (f *FS) Save(slug, messageID string, img []byte, mime string, when time.Time) (string, error) {
	if when.IsZero() {
		when = time.Now()
	}
	dir := filepath.Join(f.Root, slug, when.Format("2006"), when.Format("01"))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("creating match dir: %w", err)
	}
	name := safeName(messageID) + extFor(mime)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, img, 0o640); err != nil {
		return "", fmt.Errorf("writing match: %w", err)
	}
	return path, nil
}

func safeName(s string) string {
	s = unsafeName.ReplaceAllString(s, "-")
	if len(s) > 80 {
		s = s[:80]
	}
	if s == "" || s == "-" {
		return "img"
	}
	return s
}

func extFor(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}
