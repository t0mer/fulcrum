// Package match scores a query face embedding against enrolled references
// using cosine similarity. Embeddings are L2-normalized upstream (the ML
// sidecar guarantees it), so cosine reduces to a dot product.
package match

// Reference is one enrolled embedding belonging to a subject.
type Reference struct {
	SubjectID int64
	Embedding []float32
}

// Result is a matched subject and the similarity that won.
type Result struct {
	SubjectID  int64
	Similarity float64
}

// Cosine returns the dot product of two equal-length L2-normalized vectors.
// Mismatched lengths yield 0 (treated as no similarity).
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return float64(dot)
}

// Best finds the subject whose closest reference is most similar to query, but
// only if that similarity meets the subject's threshold. thresholdFor returns
// the acceptance threshold for a given subject id.
//
// The winning score is the max over all of a subject's references (not an
// average), so one strong reference is enough.
func Best(query []float32, refs []Reference, thresholdFor func(subjectID int64) float64) (Result, bool) {
	bestBySubject := make(map[int64]float64)
	for _, r := range refs {
		sim := Cosine(query, r.Embedding)
		if cur, ok := bestBySubject[r.SubjectID]; !ok || sim > cur {
			bestBySubject[r.SubjectID] = sim
		}
	}

	var winner Result
	found := false
	for subjectID, sim := range bestBySubject {
		if sim < thresholdFor(subjectID) {
			continue
		}
		if !found || sim > winner.Similarity {
			winner = Result{SubjectID: subjectID, Similarity: sim}
			found = true
		}
	}
	return winner, found
}
