package match

import (
	"math"
	"testing"
)

// norm L2-normalizes so Cosine == dot product, matching the ML sidecar output.
func norm(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	n := float32(math.Sqrt(s))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}

func TestCosineIdenticalIsOne(t *testing.T) {
	a := norm([]float32{1, 2, 3})
	if got := Cosine(a, a); math.Abs(got-1) > 1e-6 {
		t.Errorf("Cosine(a,a) = %v, want 1", got)
	}
}

func TestCosineOrthogonalIsZero(t *testing.T) {
	a := norm([]float32{1, 0})
	b := norm([]float32{0, 1})
	if got := Cosine(a, b); math.Abs(got) > 1e-6 {
		t.Errorf("Cosine orthogonal = %v, want 0", got)
	}
}

func TestCosineLengthMismatchIsZero(t *testing.T) {
	if got := Cosine([]float32{1, 2}, []float32{1}); got != 0 {
		t.Errorf("mismatched length = %v, want 0", got)
	}
}

func TestBestPicksArgmaxAboveThreshold(t *testing.T) {
	query := norm([]float32{1, 1, 0})
	refs := []Reference{
		{SubjectID: 1, Embedding: norm([]float32{1, 0.9, 0})}, // very close
		{SubjectID: 2, Embedding: norm([]float32{0, 0, 1})},   // far
	}
	res, ok := Best(query, refs, func(int64) float64 { return 0.5 })
	if !ok {
		t.Fatal("expected a match")
	}
	if res.SubjectID != 1 {
		t.Errorf("subject = %d, want 1", res.SubjectID)
	}
}

func TestBestRespectsThreshold(t *testing.T) {
	query := norm([]float32{1, 0})
	refs := []Reference{{SubjectID: 1, Embedding: norm([]float32{0.6, 0.8})}} // sim 0.6
	if _, ok := Best(query, refs, func(int64) float64 { return 0.9 }); ok {
		t.Error("similarity below threshold should not match")
	}
	if _, ok := Best(query, refs, func(int64) float64 { return 0.5 }); !ok {
		t.Error("similarity above threshold should match")
	}
}

func TestBestUsesMaxAcrossReferences(t *testing.T) {
	query := norm([]float32{1, 0})
	refs := []Reference{
		{SubjectID: 1, Embedding: norm([]float32{0, 1})},   // sim 0
		{SubjectID: 1, Embedding: norm([]float32{1, 0.1})}, // sim ~0.995
	}
	res, ok := Best(query, refs, func(int64) float64 { return 0.7 })
	if !ok || res.SubjectID != 1 {
		t.Fatalf("expected subject 1 via its best reference, got ok=%v res=%+v", ok, res)
	}
}
