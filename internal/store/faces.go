package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Face is a stored reference embedding for a subject.
type Face struct {
	ID         int64     `json:"id"`
	SubjectID  int64     `json:"subject_id"`
	Embedding  []float32 `json:"-"`
	SourcePath string    `json:"source_path"`
	AddedAt    string    `json:"added_at"`
}

// AddFace stores a reference embedding and its on-disk source path.
func (s *Store) AddFace(subjectID int64, embedding []float32, sourcePath string) (*Face, error) {
	res, err := s.db.Exec(
		`INSERT INTO subject_faces (subject_id, embedding, source_path) VALUES (?, ?, ?)`,
		subjectID, EncodeEmbedding(embedding), sourcePath,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting face: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Face{ID: id, SubjectID: subjectID, Embedding: embedding, SourcePath: sourcePath}, nil
}

// ListFaces returns a subject's reference faces.
func (s *Store) ListFaces(subjectID int64) ([]Face, error) {
	rows, err := s.db.Query(
		`SELECT id, subject_id, embedding, source_path, added_at
		   FROM subject_faces WHERE subject_id = ? ORDER BY added_at`, subjectID)
	if err != nil {
		return nil, fmt.Errorf("listing faces: %w", err)
	}
	defer rows.Close()

	var out []Face
	for rows.Next() {
		f, err := scanFace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// GetFace fetches one face by id.
func (s *Store) GetFace(id int64) (*Face, error) {
	row := s.db.QueryRow(
		`SELECT id, subject_id, embedding, source_path, added_at FROM subject_faces WHERE id = ?`, id)
	return scanFace(row)
}

// DeleteFace removes a face row and returns it so the caller can delete the file.
func (s *Store) DeleteFace(id int64) (*Face, error) {
	f, err := s.GetFace(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM subject_faces WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("deleting face: %w", err)
	}
	return f, nil
}

// ReplaceFaces atomically swaps a subject's faces for a new set (used by
// re-embed). Each pair is (embedding, sourcePath).
func (s *Store) ReplaceFaces(subjectID int64, embeddings [][]float32, sourcePaths []string) error {
	if len(embeddings) != len(sourcePaths) {
		return fmt.Errorf("embeddings/paths length mismatch")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM subject_faces WHERE subject_id = ?`, subjectID); err != nil {
		return fmt.Errorf("clearing faces: %w", err)
	}
	for i := range embeddings {
		if _, err := tx.Exec(
			`INSERT INTO subject_faces (subject_id, embedding, source_path) VALUES (?, ?, ?)`,
			subjectID, EncodeEmbedding(embeddings[i]), sourcePaths[i],
		); err != nil {
			return fmt.Errorf("inserting face: %w", err)
		}
	}
	return tx.Commit()
}

// AllFaces returns every reference face across all subjects (used by the matcher).
func (s *Store) AllFaces() ([]Face, error) {
	rows, err := s.db.Query(
		`SELECT id, subject_id, embedding, source_path, added_at FROM subject_faces`)
	if err != nil {
		return nil, fmt.Errorf("listing all faces: %w", err)
	}
	defer rows.Close()

	var out []Face
	for rows.Next() {
		f, err := scanFace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

func scanFace(r rowScanner) (*Face, error) {
	var f Face
	var blob []byte
	if err := r.Scan(&f.ID, &f.SubjectID, &blob, &f.SourcePath, &f.AddedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning face: %w", err)
	}
	emb, err := DecodeEmbedding(blob)
	if err != nil {
		return nil, err
	}
	f.Embedding = emb
	return &f, nil
}
