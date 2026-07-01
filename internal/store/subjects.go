package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// Subject is an enrolled child.
type Subject struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Threshold *float64  `json:"threshold,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateSubject inserts a subject. Name and slug must be unique.
func (s *Store) CreateSubject(name, slug string, threshold *float64) (*Subject, error) {
	res, err := s.db.Exec(
		`INSERT INTO subjects (name, slug, threshold) VALUES (?, ?, ?)`,
		name, slug, threshold,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting subject: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetSubject(id)
}

// GetSubject fetches one subject by id.
func (s *Store) GetSubject(id int64) (*Subject, error) {
	row := s.db.QueryRow(
		`SELECT id, name, slug, threshold, created_at FROM subjects WHERE id = ?`, id)
	return scanSubject(row)
}

// ListSubjects returns all subjects ordered by name.
func (s *Store) ListSubjects() ([]Subject, error) {
	rows, err := s.db.Query(
		`SELECT id, name, slug, threshold, created_at FROM subjects ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing subjects: %w", err)
	}
	defer rows.Close()

	var out []Subject
	for rows.Next() {
		sub, err := scanSubject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sub)
	}
	return out, rows.Err()
}

// UpdateSubject changes the display name and/or threshold. The slug is
// immutable and never updated. A nil field is left unchanged.
func (s *Store) UpdateSubject(id int64, name *string, threshold *float64, clearThreshold bool) (*Subject, error) {
	cur, err := s.GetSubject(id)
	if err != nil {
		return nil, err
	}
	newName := cur.Name
	if name != nil {
		newName = *name
	}
	newThreshold := cur.Threshold
	switch {
	case clearThreshold:
		newThreshold = nil
	case threshold != nil:
		newThreshold = threshold
	}
	if _, err := s.db.Exec(
		`UPDATE subjects SET name = ?, threshold = ? WHERE id = ?`,
		newName, newThreshold, id,
	); err != nil {
		return nil, fmt.Errorf("updating subject: %w", err)
	}
	return s.GetSubject(id)
}

// DeleteSubject removes a subject; ON DELETE CASCADE clears its faces and
// matches. Returns the deleted subject so the caller can clean up files.
func (s *Store) DeleteSubject(id int64) (*Subject, error) {
	sub, err := s.GetSubject(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM subjects WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("deleting subject: %w", err)
	}
	return sub, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubject(r rowScanner) (*Subject, error) {
	var sub Subject
	var threshold sql.NullFloat64
	if err := r.Scan(&sub.ID, &sub.Name, &sub.Slug, &threshold, &sub.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning subject: %w", err)
	}
	if threshold.Valid {
		sub.Threshold = &threshold.Float64
	}
	return &sub, nil
}
