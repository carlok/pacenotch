#!/usr/bin/env bash
# Coverage gate: reads a Go coverage profile (text format, e.g. from `go tool covdata textfmt`)
# and fails when a rule in the thresholds file is below its minimum statement coverage.
#
#   scripts/covergate.sh PROFILE [THRESHOLDS]
#
# `go tool cover -func` only prints per-function percentages, which cannot be aggregated
# into per-package statement coverage, so the gate reads the profile it is computed from.
# Duplicate blocks (unit + end-to-end runs) count as covered when any run covered them.
set -euo pipefail

profile=${1:?usage: covergate.sh PROFILE [THRESHOLDS]}
rules=${2:-$(dirname "$0")/coverage-thresholds.txt}
module=$(awk '$1 == "module" { print $2 "/"; exit }' "$(dirname "$0")/../go.mod")

awk -v module="$module" '
NR == FNR {
  if ($0 ~ /^[ \t]*(#|$)/) next
  n++; name[n] = $1; min[n] = $2; inc[n] = $3; exc[n] = (NF >= 4 ? $4 : "")
  next
}
/^mode:/ { next }
NF == 3 {
  k = $1
  if (!(k in stmts) || $2 > stmts[k]) stmts[k] = $2
  if ($3 + 0 > 0) hit[k] = 1
}
END {
  for (k in stmts) {
    f = k; sub(/:[0-9.,]+$/, "", f)
    if (index(f, module) == 1) f = substr(f, length(module) + 1)
    for (i = 1; i <= n; i++)
      if (f ~ inc[i] && (exc[i] == "" || f !~ exc[i])) {
        tot[i] += stmts[k]
        if (k in hit) cov[i] += stmts[k]
      }
  }
  fail = 0
  printf "%-32s %9s %9s  %s\n", "rule", "coverage", "minimum", "result"
  for (i = 1; i <= n; i++) {
    if (tot[i] == 0) { printf "%-32s %9s %8s%%  skipped (no statements on this OS/build)\n", name[i], "-", min[i]; continue }
    ok = (cov[i] * 100 >= min[i] * tot[i])
    printf "%-32s %8.1f%% %8s%%  %s (%d/%d statements)\n", name[i], 100 * cov[i] / tot[i], min[i], ok ? "ok" : "FAIL", cov[i], tot[i]
    if (!ok) fail = 1
  }
  exit fail
}
' "$rules" "$profile"
