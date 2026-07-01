-- 0003_perceptual_hashes: perceptual (dHash) fingerprints of processed images,
-- so near-identical re-encodes are recognized as duplicates (CLAUDE.md §10).
-- The hash is a 64-bit dHash stored as its signed-int64 bit pattern.

CREATE TABLE IF NOT EXISTS perceptual_hashes (
    hash       INTEGER PRIMARY KEY,
    first_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
