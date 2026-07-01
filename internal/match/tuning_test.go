package match

import "testing"

func TestSuggestNoConfirmedData(t *testing.T) {
	if s := SuggestThreshold(0, 0, 0, 0); s.HasSuggestion {
		t.Error("no confirmed samples should yield no suggestion")
	}
}

func TestSuggestNoRejectsSitsBelowMinConfirmed(t *testing.T) {
	s := SuggestThreshold(0.60, 0, 3, 0)
	if !s.HasSuggestion || s.Overlap {
		t.Fatalf("unexpected: %+v", s)
	}
	if s.Threshold != 0.58 {
		t.Errorf("threshold = %v, want 0.58", s.Threshold)
	}
}

func TestSuggestCleanGapMidpoint(t *testing.T) {
	s := SuggestThreshold(0.60, 0.40, 5, 4)
	if !s.HasSuggestion || s.Overlap {
		t.Fatalf("unexpected: %+v", s)
	}
	if s.Threshold != 0.50 {
		t.Errorf("threshold = %v, want 0.50 (midpoint)", s.Threshold)
	}
}

func TestSuggestOverlapFlagged(t *testing.T) {
	// A rejected face scored above the lowest confirmed one.
	s := SuggestThreshold(0.50, 0.55, 4, 3)
	if !s.HasSuggestion || !s.Overlap {
		t.Fatalf("expected overlap flagged: %+v", s)
	}
	if s.Threshold != 0.50 {
		t.Errorf("threshold = %v, want 0.50 (min confirmed)", s.Threshold)
	}
}

func TestSuggestClamps(t *testing.T) {
	if s := SuggestThreshold(0.005, 0, 1, 0); s.Threshold < 0.01 {
		t.Errorf("threshold should clamp to >= 0.01, got %v", s.Threshold)
	}
}
