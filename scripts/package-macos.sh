#!/usr/bin/env bash
# macOS GUI build: a universal (arm64 + amd64) binary and pacenotch.app (LSUIElement: no
# Dock icon), ad-hoc signed and zipped. Runs on macOS only (cgo, lipo, iconutil, codesign).
#
#   dist/pacenotch-gui_darwin_universal      CLI + `pacenotch gui`
#   dist/pacenotch-gui_macos_universal.zip   pacenotch.app
set -euo pipefail
cd "$(dirname "$0")/.."

version=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
plist_version=0.0.0
if [[ $version =~ ^v?([0-9]+\.[0-9]+\.[0-9]+) ]]; then plist_version=${BASH_REMATCH[1]}; fi

build=build/macos
rm -rf "$build"
mkdir -p "$build" dist
for arch in arm64 amd64; do
  CGO_ENABLED=1 GOOS=darwin GOARCH=$arch go build -tags gui -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$build/pacenotch-$arch" ./cmd/pacenotch
done
lipo -create -output "$build/pacenotch" "$build/pacenotch-arm64" "$build/pacenotch-amd64"

app=$build/pacenotch.app
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$build/pacenotch" "$app/Contents/MacOS/pacenotch"
sed "s/@VERSION@/$plist_version/g" packaging/macos/Info.plist > "$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist" >/dev/null

# the icon is rendered by the same bar rasterizer as the tray and the window
iconset=$(pwd)/$build/pacenotch.iconset
PACENOTCH_ICONSET=$iconset go test -count=1 ./internal/gui -run '^TestWriteIconset$' >/dev/null
iconutil -c icns -o "$app/Contents/Resources/pacenotch.icns" "$iconset"

# ad-hoc signature (not notarized): Apple Silicon refuses unsigned code
codesign --force --deep --sign - "$app"
codesign --force --sign - "$build/pacenotch"

cp "$build/pacenotch" dist/pacenotch-gui_darwin_universal
rm -f dist/pacenotch-gui_macos_universal.zip
ditto -c -k --keepParent "$app" dist/pacenotch-gui_macos_universal.zip
lipo -info dist/pacenotch-gui_darwin_universal
echo "built dist/pacenotch-gui_darwin_universal and dist/pacenotch-gui_macos_universal.zip ($version)"
