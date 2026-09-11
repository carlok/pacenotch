#!/usr/bin/env bash
# Cross-compiles the CLI-only build (pure Go, no cgo) into dist/.
set -euo pipefail
cd "$(dirname "$0")/.."
version=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
mkdir -p dist
for target in darwin/arm64 darwin/amd64 linux/amd64 windows/amd64; do
  os=${target%/*} arch=${target#*/}
  ext=""; [[ $os == windows ]] && ext=.exe
  outfile=dist/pacenotch_${os}_${arch}${ext}
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$outfile" ./cmd/pacenotch
  echo "built $outfile"
done
