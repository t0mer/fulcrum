package match

// Suggestion is a threshold recommendation derived from review history.
type Suggestion struct {
	Threshold float64 `json:"threshold"`
	// HasSuggestion is false when there isn't enough data to advise.
	HasSuggestion bool `json:"has_suggestion"`
	// Overlap is true when a rejected face scored at or above the lowest
	// confirmed one — the two classes can't be cleanly separated, so any
	// threshold will misclassify some samples.
	Overlap bool `json:"overlap"`
}

// SuggestThreshold recommends a threshold from the lowest confirmed similarity
// and the highest rejected similarity. It needs at least one confirmed sample.
//
//   - No rejects: sit just below the lowest confirmed score (keep recall).
//   - Clean gap: sit in the middle of the gap.
//   - Overlap: fall back to the lowest confirmed score and flag the overlap.
func SuggestThreshold(minConfirmed, maxRejected float64, confirmedCount, rejectedCount int) Suggestion {
	if confirmedCount == 0 {
		return Suggestion{}
	}
	switch {
	case rejectedCount == 0:
		return Suggestion{Threshold: clamp(minConfirmed - 0.02), HasSuggestion: true}
	case maxRejected < minConfirmed:
		return Suggestion{Threshold: clamp((maxRejected + minConfirmed) / 2), HasSuggestion: true}
	default:
		return Suggestion{Threshold: clamp(minConfirmed), HasSuggestion: true, Overlap: true}
	}
}

func clamp(v float64) float64 {
	if v < 0.01 {
		return 0.01
	}
	if v > 0.99 {
		return 0.99
	}
	// round to 2 decimals for a clean UI value
	return float64(int(v*100+0.5)) / 100
}
