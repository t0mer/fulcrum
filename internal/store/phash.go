package store

import (
	"fmt"
	"math/bits"
)

// SeenSimilarImage reports whether a perceptual hash within maxDistance of a
// previously-seen image exists. If none does, the hash is recorded and the
// call returns fresh=true. Comparison is done in Go (Hamming distance), which
// is fine at home-lab volume.
func (s *Store) SeenSimilarImage(hash uint64, maxDistance int) (fresh bool, err error) {
	rows, err := s.db.Query(`SELECT hash FROM perceptual_hashes`)
	if err != nil {
		return false, fmt.Errorf("reading perceptual hashes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var stored int64
		if err := rows.Scan(&stored); err != nil {
			return false, err
		}
		if bits.OnesCount64(hash^uint64(stored)) <= maxDistance {
			return false, nil // a near-duplicate already exists
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	if _, err := s.db.Exec(
		`INSERT OR IGNORE INTO perceptual_hashes (hash) VALUES (?)`, int64(hash)); err != nil {
		return false, fmt.Errorf("recording perceptual hash: %w", err)
	}
	return true, nil
}
