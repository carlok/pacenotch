#!/usr/bin/env bash
# Regenerates testdata/reference.sh from reference/claude-pace.sh for the parity tests.
# The patch changes exactly two things:
#   1. "now" comes from PACENOTCH_NOW (Unix seconds) when it is set;
#   2. the header prints "pacenotch" instead of "claude-pace".
set -euo pipefail
cd "$(dirname "$0")/.."

src=reference/claude-pace.sh
dst=testdata/reference.sh

read -r -d '' prog <<'AWK' || true
function repl(s, from, to,   i, out) {
  out = ""
  while ((i = index(s, from)) > 0) { out = out substr(s, 1, i - 1) to; s = substr(s, i + length(from)); n++ }
  return out s
}
{
  line = $0
  line = repl(line, "$(date +%s)", "$(_now)")
  line = repl(line, "now_s=$(date '+%a %d %b %H:%M')", "now_s=$(_date_now)")
  line = repl(line, "lp=\" claude-pace   $now_s\"", "lp=\" pacenotch   $now_s\"")
  line = repl(line, "lc=\" ${B}claude-pace${R}   $now_s\"", "lc=\" ${B}pacenotch${R}   $now_s\"")
  print line
  if (index(line, "isnum() {") == 1) {
    print "_now() { if [[ -n ${PACENOTCH_NOW:-} ]]; then echo \"$PACENOTCH_NOW\"; else date +%s; fi; }"
    print "_date_now() { local t; t=$(_now); date -r \"$t\" '+%a %d %b %H:%M' 2>/dev/null || date -d \"@$t\" '+%a %d %b %H:%M'; }"
    added = 1
  }
}
END {
  # 4x $(date +%s), 1x header date, 2x program name
  if (n != 7 || !added) { printf "patch-reference: expected 7 replacements, got %d\n", n > "/dev/stderr"; exit 1 }
}
AWK

mkdir -p testdata
awk "$prog" "$src" > "$dst.tmp"
mv "$dst.tmp" "$dst"
chmod +x "$dst"
echo "wrote $dst"
