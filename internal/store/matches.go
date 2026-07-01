package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Match records an image where an enrolled subject was detected.
type Match struct {
	ID            int64     `json:"id"`
	MessageID     string    `json:"message_id"`
	ImageSHA256   string    `json:"image_sha256"`
	SubjectID     int64     `json:"subject_id"`
	SubjectName   string    `json:"subject_name"`
	SubjectSlug   string    `json:"subject_slug"`
	Similarity    float64   `json:"similarity"`
	SourceGroupID string    `json:"source_group_id"`
	SourceGroup   string    `json:"source_group"`
	StoredPath    string    `json:"stored_path"`
	Forwarded     bool      `json:"forwarded"`
	Reviewed      string    `json:"reviewed"`
	CreatedAt     time.Time `json:"created_at"`
	Embedding     []float32 `json:"-"` // matched face; used to reinforce on confirm
}

// CreateMatch inserts a match, ignoring the (message_id, subject) duplicate.
// Returns the new id, or 0 if it already existed.
func (s *Store) CreateMatch(m Match) (int64, error) {
	var emb []byte
	if len(m.Embedding) > 0 {
		emb = EncodeEmbedding(m.Embedding)
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO matches
		   (message_id, image_sha256, subject_id, similarity, source_group_id, stored_path, forwarded, embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.MessageID, m.ImageSHA256, m.SubjectID, m.Similarity, m.SourceGroupID, m.StoredPath, m.Forwarded, emb,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting match: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// MatchFilter narrows a match listing.
type MatchFilter struct {
	SubjectID int64  // 0 = any
	Reviewed  string // "" = any
}

// ListMatches returns matches (most recent first) joined with subject/group names.
func (s *Store) ListMatches(f MatchFilter) ([]Match, error) {
	var where []string
	var args []any
	if f.SubjectID != 0 {
		where = append(where, "m.subject_id = ?")
		args = append(args, f.SubjectID)
	}
	if f.Reviewed != "" {
		where = append(where, "m.reviewed = ?")
		args = append(args, f.Reviewed)
	}
	q := `SELECT m.id, m.message_id, m.image_sha256, m.subject_id,
	             s.name, s.slug, m.similarity, m.source_group_id,
	             COALESCE(g.name, m.source_group_id), m.stored_path,
	             m.forwarded, m.reviewed, m.created_at
	        FROM matches m
	        JOIN subjects s ON s.id = m.subject_id
	   LEFT JOIN groups g ON g.provider_group_id = m.source_group_id`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY m.created_at DESC, m.id DESC"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing matches: %w", err)
	}
	defer rows.Close()

	var out []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.ID, &m.MessageID, &m.ImageSHA256, &m.SubjectID,
			&m.SubjectName, &m.SubjectSlug, &m.Similarity, &m.SourceGroupID,
			&m.SourceGroup, &m.StoredPath, &m.Forwarded, &m.Reviewed, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning match: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMatch fetches one match with joined names (including the matched embedding).
func (s *Store) GetMatch(id int64) (*Match, error) {
	var m Match
	var emb []byte
	err := s.db.QueryRow(
		`SELECT m.id, m.message_id, m.image_sha256, m.subject_id,
		        s.name, s.slug, m.similarity, m.source_group_id,
		        COALESCE(g.name, m.source_group_id), m.stored_path,
		        m.forwarded, m.reviewed, m.created_at, m.embedding
		   FROM matches m
		   JOIN subjects s ON s.id = m.subject_id
	  LEFT JOIN groups g ON g.provider_group_id = m.source_group_id
		  WHERE m.id = ?`, id).
		Scan(&m.ID, &m.MessageID, &m.ImageSHA256, &m.SubjectID,
			&m.SubjectName, &m.SubjectSlug, &m.Similarity, &m.SourceGroupID,
			&m.SourceGroup, &m.StoredPath, &m.Forwarded, &m.Reviewed, &m.CreatedAt, &emb)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting match: %w", err)
	}
	if len(emb) > 0 {
		if e, derr := DecodeEmbedding(emb); derr == nil {
			m.Embedding = e
		}
	}
	return &m, nil
}

// RecordHardNegative stores a rejected match's embedding as a false-positive
// example for later threshold tuning.
func (s *Store) RecordHardNegative(subjectID int64, embedding []float32, similarity float64) error {
	if len(embedding) == 0 {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO hard_negatives (subject_id, embedding, similarity) VALUES (?, ?, ?)`,
		subjectID, EncodeEmbedding(embedding), similarity,
	)
	if err != nil {
		return fmt.Errorf("recording hard negative: %w", err)
	}
	return nil
}

// SetReviewed marks a match confirmed or rejected.
func (s *Store) SetReviewed(id int64, status string) error {
	res, err := s.db.Exec(`UPDATE matches SET reviewed = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("reviewing match: %w", err)
	}
	return affected(res)
}

// SetForwarded flags a match as forwarded to the destination group.
func (s *Store) SetForwarded(id int64) error {
	_, err := s.db.Exec(`UPDATE matches SET forwarded = 1 WHERE id = ?`, id)
	return err
}
