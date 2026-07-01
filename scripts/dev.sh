#!/usr/bin/env bash
# Local dev: run the Go backend against a local SQLite db and data dir.
# Frontend hot-reload (Vite) is added once the web/ scaffold lands; until then
# the embedded placeholder SPA is served.
set -euo pipefail

DATA_DIR="${FULCRUM_DATA_DIR:-./.dev-data}"
mkdir -p "$DATA_DIR/faces" "$DATA_DIR/matches"

export FULCRUM_DB_PATH="${FULCRUM_DB_PATH:-$DATA_DIR/fulcrum.db}"
export FULCRUM_ENROLL_FACES_PATH="${FULCRUM_ENROLL_FACES_PATH:-$DATA_DIR/faces}"
export FULCRUM_SINK_STORAGE_PATH="${FULCRUM_SINK_STORAGE_PATH:-$DATA_DIR/matches}"
export FULCRUM_SERVER_LOG_LEVEL="${FULCRUM_SERVER_LOG_LEVEL:-debug}"

exec go run ./cmd/fulcrum "$@"
