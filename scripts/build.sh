#!/usr/bin/env bash
# Cross-compile the fulcrum Go binary into dist/ for all release targets.
# The frontend must be built first (scripts/build-web.sh or `cd web && npm run
# build`) so the embedded SPA is current; this script embeds whatever is in
# web/dist.
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X github.com/t0mer/fulcrum/internal/version.Version=${VERSION}"
OUT="dist"
mkdir -p "$OUT"

# OS/ARCH[/ARM] targets. The Go binary is scratch-friendly on all of these;
# the fulcrum-ml sidecar is amd64/arm64 only (see CLAUDE.md §3).
targets=(
  "linux/amd64"
  "linux/arm64"
  "linux/arm/7"
)

for t in "${targets[@]}"; do
  IFS=/ read -r GOOS GOARCH GOARM <<<"$t"
  name="fulcrum_${GOOS}_${GOARCH}${GOARM:+v$GOARM}"
  echo "building $name (version $VERSION)"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" GOARM="${GOARM:-}" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/$name" ./cmd/fulcrum
done

echo "done -> $OUT/"
