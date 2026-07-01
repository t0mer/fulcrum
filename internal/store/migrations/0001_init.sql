-- 0001_init: baseline schema (see CLAUDE.md §14).

CREATE TABLE IF NOT EXISTS subjects (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,          -- display name, may be Hebrew/RTL
    slug       TEXT NOT NULL UNIQUE,          -- ^[a-z0-9-]{1,32}$, on-disk folder name
    threshold  REAL,                          -- NULL => use global default
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS subject_faces (
    id          INTEGER PRIMARY KEY,
    subject_id  INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    embedding   BLOB NOT NULL,                -- 512 float32, L2-normalized
    source_path TEXT NOT NULL,
    added_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_subject_faces_subject ON subject_faces(subject_id);

CREATE TABLE IF NOT EXISTS groups (
    id                INTEGER PRIMARY KEY,
    provider_group_id TEXT NOT NULL UNIQUE,
    name              TEXT NOT NULL,
    monitored         BOOLEAN NOT NULL DEFAULT 0,
    is_destination    BOOLEAN NOT NULL DEFAULT 0,
    last_seen         TIMESTAMP
);

CREATE TABLE IF NOT EXISTS jobs (
    id                INTEGER PRIMARY KEY,
    provider          TEXT NOT NULL,
    provider_group_id TEXT NOT NULL,
    message_id        TEXT NOT NULL,
    media_ref         TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending', -- pending|processing|done|failed|dead
    attempts          INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);

CREATE TABLE IF NOT EXISTS seen_media (
    sha256     TEXT PRIMARY KEY,
    first_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS matches (
    id              INTEGER PRIMARY KEY,
    message_id      TEXT NOT NULL,
    image_sha256    TEXT NOT NULL,
    subject_id      INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    similarity      REAL NOT NULL,
    source_group_id TEXT NOT NULL,
    stored_path     TEXT,
    forwarded       BOOLEAN NOT NULL DEFAULT 0,
    reviewed        TEXT NOT NULL DEFAULT 'unreviewed', -- unreviewed|confirmed|rejected
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(message_id, subject_id)
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT
);
