package store

import "fmt"

// MarkSeen records a media sha256 and reports whether it was newly seen.
// A false return means the image was already processed (dedup hit).
func (s *Store) MarkSeen(sha256 string) (fresh bool, err error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO seen_media (sha256) VALUES (?)`, sha256)
	if err != nil {
		return false, fmt.Errorf("marking media seen: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
