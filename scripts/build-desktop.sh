#!/usr/bin/env bash
# Build the desktop app for the current OS into release/.
# Windows users should run scripts/build-desktop.ps1 instead (NSIS + signing).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
frontend="$root/desktop/frontend"
app="$root/cmd/rta-excel-filler"
release="$root/release"
goos="${GOOS:-$(go env GOOS)}"
goarch="${GOARCH:-$(go env GOARCH)}"

if ! command -v go >/dev/null; then
  echo "Go is not on PATH" >&2
  exit 1
fi
if ! command -v bun >/dev/null; then
  echo "Bun is not on PATH" >&2
  exit 1
fi

mkdir -p "$release"
(
  cd "$frontend"
  bun install --frozen-lockfile
  bun run build
)

name="RTA-Excel-Filler-${goos}-${goarch}"
out="$release/$name"
if [ "$goos" = windows ]; then
  out="${out}.exe"
fi

(
  cd "$app"
  CGO_ENABLED=1 go build -tags production -trimpath -ldflags '-w -s' -o "$out" .
)

if [ ! -f "$out" ]; then
  echo "go build did not produce $out" >&2
  exit 1
fi

(
  cd "$release"
  if command -v sha256sum >/dev/null; then
    sha256sum "$(basename "$out")" > SHA256SUMS.txt
  else
    shasum -a 256 "$(basename "$out")" > SHA256SUMS.txt
  fi
)

echo "Desktop artifact written to $out"
