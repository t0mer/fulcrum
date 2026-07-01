package store

import (
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Every migration table should exist and be queryable.
	for _, tbl := range []string{"subjects", "subject_faces", "groups", "jobs", "seen_media", "matches", "settings"} {
		if _, err := s.DB().Exec("SELECT COUNT(1) FROM " + tbl); err != nil {
			t.Errorf("table %q not created: %v", tbl, err)
		}
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s1.Close()

	// Re-opening the same file must not re-apply migrations or error.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	var applied int
	if err := s2.DB().QueryRow("SELECT COUNT(1) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("counting migrations: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied migrations = %d, want 1", applied)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// subject_faces.subject_id references a non-existent subject -> should fail.
	_, err = s.DB().Exec(`INSERT INTO subject_faces (subject_id, embedding, source_path) VALUES (999, x'00', '/x')`)
	if err == nil {
		t.Fatal("expected foreign-key violation, got nil")
	}
}
