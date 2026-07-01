package store

import "database/sql"

// TuningStats summarizes the review history for one subject: the lowest
// similarity we accepted (confirmed) versus the highest we rejected. The gap
// between them is where a good threshold sits.
type TuningStats struct {
	SubjectID      int64   `json:"subject_id"`
	ConfirmedCount int     `json:"confirmed_count"`
	RejectedCount  int     `json:"rejected_count"`
	MinConfirmed   float64 `json:"min_confirmed"` // 0 if none
	MaxRejected    float64 `json:"max_rejected"`  // 0 if none
}

// TuningStatsFor gathers the review history for a subject. Confirmed scores
// come from reviewed matches; rejected scores come from recorded hard-negatives.
func (s *Store) TuningStatsFor(subjectID int64) (TuningStats, error) {
	st := TuningStats{SubjectID: subjectID}

	var minConf sql.NullFloat64
	if err := s.db.QueryRow(
		`SELECT COUNT(1), MIN(similarity) FROM matches WHERE subject_id = ? AND reviewed = 'confirmed'`,
		subjectID).Scan(&st.ConfirmedCount, &minConf); err != nil {
		return st, err
	}
	if minConf.Valid {
		st.MinConfirmed = minConf.Float64
	}

	var maxRej sql.NullFloat64
	if err := s.db.QueryRow(
		`SELECT COUNT(1), MAX(similarity) FROM hard_negatives WHERE subject_id = ?`,
		subjectID).Scan(&st.RejectedCount, &maxRej); err != nil {
		return st, err
	}
	if maxRej.Valid {
		st.MaxRejected = maxRej.Float64
	}
	return st, nil
}
