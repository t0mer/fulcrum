package store

import (
	"path/filepath"
	"testing"
)

func TestSeenSimilarImage(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	const base = uint64(0xF0F0F0F0F0F0F0F0)

	// First sighting is fresh and gets recorded.
	fresh, err := s.SeenSimilarImage(base, 4)
	if err != nil || !fresh {
		t.Fatalf("first = %v, %v; want fresh", fresh, err)
	}
	// Exact same hash -> not fresh.
	if fresh, _ := s.SeenSimilarImage(base, 4); fresh {
		t.Error("identical hash should not be fresh")
	}
	// Within distance (flip 2 bits) -> not fresh.
	near := base ^ 0b11
	if fresh, _ := s.SeenSimilarImage(near, 4); fresh {
		t.Error("near hash within distance should not be fresh")
	}
	// Far away (flip 20 bits) -> fresh.
	far := base ^ 0xFFFFF
	if fresh, _ := s.SeenSimilarImage(far, 4); !fresh {
		t.Error("distant hash should be fresh")
	}
}
