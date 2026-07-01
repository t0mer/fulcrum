-- 0002_active_learning: keep the matched embedding so a confirmed match can
-- reinforce the subject's references, and record rejected embeddings as
-- hard-negatives for threshold tuning (see CLAUDE.md §11).

ALTER TABLE matches ADD COLUMN embedding BLOB;

CREATE TABLE IF NOT EXISTS hard_negatives (
    id         INTEGER PRIMARY KEY,
    subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    embedding  BLOB NOT NULL,            -- 512 float32, L2-normalized
    similarity REAL NOT NULL,            -- score that triggered the false match
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_hard_negatives_subject ON hard_negatives(subject_id);
