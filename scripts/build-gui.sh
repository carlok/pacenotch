#!/usr/bin/env bash
# Native GUI build for the current OS into dist/ (cgo: run it on each OS, as CI does).
set -euo pipefail
cd "$(dirname "$0")/.."
version=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
ldflags="-s -w -X main.version=$version"
mkdir -p dist
case $(go env GOOS) in
darwin)
  scripts/package-macos.sh
  ;;
linux)
  go build -tags gui -trimpath -ldflags "$ldflags" -o dist/pacenotch-gui_linux_amd64 ./cmd/pacenotch
  echo "built dist/pacenotch-gui_linux_amd64 (CLI + pacenotch gui)"
  ;;
windows)
  # one exe cannot be both a console and a GUI app: the CLI is pacenotch.exe (CLI-only build)
  go build -tags gui -trimpath -ldflags "$ldflags -H=windowsgui" -o dist/pacenotch-gui_windows_amd64.exe ./cmd/pacenotch-gui
  echo "built dist/pacenotch-gui_windows_amd64.exe"
  ;;
*)
  echo "build-gui: unsupported OS $(go env GOOS)" >&2
  exit 1
  ;;
esac
