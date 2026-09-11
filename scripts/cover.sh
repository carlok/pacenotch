#!/usr/bin/env bash
# Unit coverage (race-enabled) + end-to-end coverage of the real binary, merged with
# `go tool covdata`, then printed per function and checked by the coverage gate.
set -euo pipefail
cd "$(dirname "$0")/.."

# Native Windows paths when running under Git Bash, so go.exe understands them.
root=$(pwd -W 2>/dev/null || pwd)
out=$root/coverage
exe=""
[[ $(go env GOOS) == windows ]] && exe=.exe

rm -rf coverage
mkdir -p coverage/unit coverage/e2e coverage/merged

echo "== unit tests"
go test -race -covermode=atomic -coverprofile=coverage.out ./... -args -test.gocoverdir="$out/unit"

echo "== end-to-end tests against a coverage-instrumented binary"
go build -cover -covermode=atomic -o "coverage/pacenotch-cover$exe" ./cmd/pacenotch
PACENOTCH_E2E_BIN="$out/pacenotch-cover$exe" GOCOVERDIR="$out/e2e" \
  go test -race -count=1 ./e2e

echo "== merge"
go tool covdata merge -i="$out/unit,$out/e2e" -o "$out/merged"
go tool covdata textfmt -i="$out/merged" -o "$out/merged.out"

echo "== per package (merged)"
go tool covdata percent -i="$out/merged"
echo "== per function (merged)"
go tool cover -func="$out/merged.out"

echo "== gate"
scripts/covergate.sh coverage/merged.out
