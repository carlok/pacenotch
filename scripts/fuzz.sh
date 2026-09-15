#!/usr/bin/env bash
# Runs every fuzz target for a short time (default 10s; CI passes 30s).
# Crashers land in <package>/testdata/fuzz/<Target>/ and must be committed as regression cases.
set -euo pipefail
cd "$(dirname "$0")/.."
t=${1:-10s}
for target in internal/usage:FuzzParseResetsAt internal/usage:FuzzParse internal/pace:FuzzRows internal/update:FuzzParseSemver; do
  pkg=${target%%:*} name=${target##*:}
  echo "== $pkg $name ($t)"
  go test "./$pkg" -run '^$' -fuzz "^$name\$" -fuzztime "$t"
done
