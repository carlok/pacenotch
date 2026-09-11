#!/usr/bin/env bash
# claude-pace — Claude plan usage vs. linear pace, with width-fitting bars.
# Works with macOS bash 3.2. Needs: curl, jq (1.6+).
#
#   claude-pace                 full view
#   claude-pace -c              compact: one line per window (automatic below 60 cols)
#   claude-pace -w [SECS]       watch mode (default refresh 60s), redraws on resize
#   claude-pace -b 3            "on pace" band in points (default 5)
#   claude-pace --from FILE     read JSON from FILE ('-' = stdin) instead of the API;
#                               accepts the API format or the statusline rate_limits format
#   claude-pace --raw           print the raw JSON and exit
#   options: --ttl SECS (cache, default 60) --width N --color auto|always|never
#
# Exit code (single-shot mode): 0 ok, 1 error, 2 weekly (all models) is ahead of pace.
# Uses the undocumented /api/oauth/usage endpoint: it may change or rate-limit (429).

set -uo pipefail

BAND=5 TTL=60 WATCH=0 INTERVAL=60 COMPACT=0 RAW=0 FROM="" WIDTH="" COLOR=auto
CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/claude-pace.json"
ERR="" WARN="" STALE=0

usage() { sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }
die() { printf 'claude-pace: %s\n' "$*" >&2; exit 1; }
isnum() { [[ $1 =~ ^[0-9]+([.][0-9]+)?$ ]]; }

while (( $# )); do
  case $1 in
    -c|--compact) COMPACT=1 ;;
    -w|--watch)   WATCH=1; if [[ ${2:-} =~ ^[0-9]+$ ]]; then INTERVAL=$2; shift; fi ;;
    -b|--band)    isnum "${2:-}" || die "--band needs a number"; BAND=$2; shift ;;
    --ttl)        [[ ${2:-} =~ ^[0-9]+$ ]] || die "--ttl needs seconds"; TTL=$2; shift ;;
    --width)      [[ ${2:-} =~ ^[0-9]+$ ]] || die "--width needs a number"; WIDTH=$2; shift ;;
    --from)       [[ -n ${2:-} ]] || die "--from needs a file"; FROM=$2; shift ;;
    --raw)        RAW=1 ;;
    --color)      COLOR=${2:-auto}; shift ;;
    --color=*)    COLOR=${1#--color=} ;;
    --no-color)   COLOR=never ;;
    -h|--help)    usage 0 ;;
    *)            die "unknown option: $1 (try --help)" ;;
  esac
  shift
done
command -v jq >/dev/null || die "jq not found"
command -v curl >/dev/null || die "curl not found"

# ---------- colors ----------
if [[ $COLOR == always || ( $COLOR == auto && -t 1 && -z ${NO_COLOR:-} && ${TERM:-} != dumb ) ]]; then
  R=$'\033[0m' B=$'\033[1m' D=$'\033[2m'
  FG_ahead=$'\033[31m' FG_on=$'\033[33m' FG_under=$'\033[32m'
  BG_ahead=$'\033[41m' BG_on=$'\033[43m' BG_under=$'\033[42m'
  FG_idle=$'\033[36m' BG_idle=$'\033[46m'
  C_EMPTY=$'\033[90m' C_MARK=$'\033[1;97m'
else
  R="" B="" D="" FG_ahead="" FG_on="" FG_under="" BG_ahead="" BG_on="" BG_under="" C_EMPTY="" C_MARK=""
  FG_idle="" BG_idle=""
fi
PART=("" "▏" "▎" "▍" "▌" "▋" "▊" "▉")

# ---------- data ----------
mtime() { stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null || echo 0; }

get_token() {   # sets TOKEN (no subshell, so ERR/WARN survive)
  TOKEN=""
  if [[ -n ${CLAUDE_PACE_TOKEN:-} ]]; then TOKEN=$CLAUDE_PACE_TOKEN; return 0; fi
  local creds="" exp
  if command -v security >/dev/null 2>&1; then
    creds=$(security find-generic-password -s "Claude Code-credentials" -w 2>/dev/null)
  fi
  [[ -z $creds && -r $HOME/.claude/.credentials.json ]] && creds=$(cat "$HOME/.claude/.credentials.json")
  [[ -z $creds ]] && { ERR="no Claude Code credentials found (run: claude login)"; return 1; }
  TOKEN=$(jq -r '.claudeAiOauth.accessToken // empty' <<<"$creds" 2>/dev/null)
  exp=$(jq -r '(.claudeAiOauth.expiresAt // 0) / 1000 | floor' <<<"$creds" 2>/dev/null)
  [[ -z $TOKEN ]] && { ERR="credentials found but no access token"; return 1; }
  if (( ${exp:-0} > 0 && exp < $(date +%s) )); then
    WARN="OAuth token looks expired: start Claude Code once to refresh it"
  else
    WARN=""
  fi
  return 0
}

fetch() {
  local tmp http
  get_token || return 1
  tmp=$(mktemp "${TMPDIR:-/tmp}/claude-pace.XXXXXX")
  http=$(curl -s -m 10 -o "$tmp" -w '%{http_code}' https://api.anthropic.com/api/oauth/usage \
          -H "Authorization: Bearer $TOKEN" -H "anthropic-beta: oauth-2025-04-20") || http=000
  if [[ $http == 200 ]] && jq -e 'type == "object"' "$tmp" >/dev/null 2>&1; then
    mkdir -p "$(dirname "$CACHE")" && mv "$tmp" "$CACHE" && return 0
  fi
  rm -f "$tmp"
  case $http in
    401) ERR="HTTP 401: token rejected (start Claude Code once to refresh it)" ;;
    429) ERR="HTTP 429: usage endpoint is rate limiting, try again later" ;;
    000) ERR="network error" ;;
    *)   ERR="HTTP $http from usage endpoint" ;;
  esac
  return 1
}

AGE=0
load_data() {
  STALE=0 ERR=""
  if [[ -n $FROM ]]; then
    if [[ $FROM == - ]]; then DATA=$(cat); else DATA=$(cat "$FROM") || die "cannot read $FROM"; fi
    AGE=-1; return 0
  fi
  local now; now=$(date +%s)
  AGE=$(( now - $(mtime "$CACHE") ))
  if [[ ! -s $CACHE ]] || (( AGE >= TTL )); then
    if fetch; then AGE=0
    elif [[ -s $CACHE ]]; then STALE=1; AGE=$(( now - $(mtime "$CACHE") ))
    else die "$ERR"
    fi
  fi
  DATA=$(cat "$CACHE")
}

# One TSV row per window:
# label short used_bp exp_bp used exp delta class secs_left budget unit flat proj_pct hit_in_secs
read -r -d '' JQ_ROWS <<'JQ'
def clamp($a; $b): if . < $a then $a elif . > $b then $b else . end;
def f1: (clamp(0; 1e9) * 10 | round) as $x | "\($x / 10 | floor).\($x % 10)";
def epoch:
  if type == "number" then .
  else capture("^(?<b>\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2})(\\.\\d+)?(?<tz>.*)$") as $c
    | ($c.b + "Z" | fromdateiso8601) as $t
    | if $c.tz == "" or $c.tz == "Z" then $t
      else ($c.tz | capture("^(?<s>[+-])(?<h>\\d{2}):?(?<m>\\d{2})$")) as $o
        | $t - (if $o.s == "-" then -1 else 1 end) * (($o.h|tonumber) * 3600 + ($o.m|tonumber) * 60)
      end
  end;
def win($k): if ($k|startswith("five_hour")) then 18000 elif ($k|startswith("seven_day")) then 604800 else null end;
def lbl($k): {five_hour: "5h session", seven_day: "7d all models", seven_day_sonnet: "7d Sonnet", seven_day_opus: "7d Opus"}[$k]
               // ($k | sub("^seven_day_"; "7d ") | sub("^five_hour_"; "5h "));
def short($k): {five_hour: "5h", seven_day: "7d", seven_day_sonnet: "7d S", seven_day_opus: "7d O"}[$k] // ($k | .[0:5]);

(if has("rate_limits") then .rate_limits else . end)
| to_entries
| map(select(.value | type == "object")
      | .value.u = (.value.utilization // .value.used_percentage)
      | select(.value.u != null and win(.key) != null))
| sort_by(if .key == "five_hour" then 0 elif .key == "seven_day" then 1 else 2 end)
| .[]
| .key as $k | win($k) as $w | .value.u as $u
| (if $w == 604800 then 86400 else 3600 end) as $unit
| (if $unit == 86400 then "day" else "h" end) as $uname
| (100 / ($w / $unit) | f1) as $flat
| (if .value.resets_at == null then null else (.value.resets_at | epoch) - $now end) as $left
| if $left == null or $left <= 0 then
    # no active window (never started, or already reset since the data was fetched)
    (if $left == null then $u else 0 end) as $u0
    | [lbl($k), short($k), ($u0 * 100 | round | clamp(0; 10000)), -1,
       ($u0 | round), -1, 0, "idle", -1, "", $uname, $flat, -1, -1]
  else
    (($w - $left) | clamp(0; $w)) as $el
    | ($el / $w * 100) as $exp
    | ($u - $exp) as $d
    | (if $d > $band then "ahead" elif $d >= -$band then "on" else "under" end) as $cls
    | (if $left > 0 then (100 - $u) / ($left / $unit) else 0 end) as $budget
    | (if $el >= $w * 0.05 and $u > 0 then $u / ($el / $w) else null end) as $proj
    | (if $u >= 100 then 0 elif $proj != null and $proj > 100 then ((100 - $u) / ($u / $el) | floor) else -1 end) as $hit
    | [lbl($k), short($k), ($u * 100 | round | clamp(0; 10000)), ($exp * 100 | round),
       ($u | round), ($exp | round), ($d | round), $cls, ($left | floor | clamp(0; $w)),
       ($budget | f1), $uname, $flat, ($proj // -1 | round), $hit]
  end
| @tsv
JQ

read -r -d '' JQ_EXTRA <<'JQ'
.extra_usage // {} | select(.is_enabled == true)
| "extra usage: on" + (if .utilization != null then " (\(.utilization | round)% of monthly limit)" else "" end)
JQ

# ---------- rendering ----------
term_cols() {
  local c=""
  [[ -n $WIDTH ]] && { echo "$WIDTH"; return; }
  c=$(stty size </dev/tty 2>/dev/null | awk '{print $2}')
  [[ -z $c || $c == 0 ]] && c=$(tput cols 2>/dev/null)
  [[ -z $c || $c == 0 ]] && c=${COLUMNS:-80}
  echo "$c"
}

fmt_dur() {
  local s=$1 d h m; (( s < 0 )) && s=0
  d=$(( s / 86400 )) h=$(( s % 86400 / 3600 )) m=$(( s % 3600 / 60 ))
  if (( d > 0 )); then echo "${d}d ${h}h"; elif (( h > 0 )); then echo "${h}h ${m}m"; else echo "${m}m"; fi
}

# bar WIDTH USED_BP EXP_BP CLASS  -> fill up to used (1/8-cell precision), marker at pace
bar() {
  local w=$1 ubp=$2 ebp=$3 cls=$4 fg bg eighths full rem mark i k prev="" ch out=""
  eval "fg=\$FG_$cls bg=\$BG_$cls"
  eighths=$(( ubp * w * 8 / 10000 )); full=$(( eighths / 8 )); rem=$(( eighths % 8 ))
  (( full >= w )) && { full=$w; rem=0; }
  if (( ebp < 0 )); then mark=-1
  else mark=$(( ebp * w / 10000 )); (( mark >= w )) && mark=$(( w - 1 )); fi
  for (( i = 0; i < w; i++ )); do
    if (( i == mark )); then
      if (( i < full )); then k=mi; else k=mo; fi; ch="┃"
    elif (( i < full )); then k=f; ch="█"
    elif (( i == full && rem > 0 )); then k=f; ch=${PART[$rem]}
    else k=e; ch="░"
    fi
    if [[ $k != "$prev" ]]; then
      case $k in
        f)  out+="$R$fg" ;;
        e)  out+="$R$C_EMPTY" ;;
        mi) out+="$R$bg$C_MARK" ;;
        mo) out+="$R$C_MARK" ;;
      esac
      prev=$k
    fi
    out+=$ch
  done
  printf '%s%s' "$out" "$R"
}

# left/right aligned line; plain strings are used for measuring (keep them ASCII)
lr() {
  local lp=$1 lc=$2 rp=$3 rc=$4 pad
  pad=$(( COLS - 1 - ${#lp} - ${#rp} ))
  if (( pad >= 2 )); then printf '%s%*s%s\n' "$lc" "$pad" "" "$rc"
  else printf '%s\n%s\n' "$lc" "  $rc"; fi
}

status_word() {
  case $1 in
    ahead) GLYPH="▲" WORD="slow down" ;;
    on)    GLYPH="●" WORD="on pace" ;;
    idle)  GLYPH="○" WORD="not started" ;;
    *)     GLYPH="▼" WORD="room to spare" ;;
  esac
}

render() {
  local rows extra label short ubp ebp u e d cls left budget unit flat proj hit
  local fg sd lp lc rp rc tail w pline pp pc now_s
  rows=$(jq -r --argjson now "$(date +%s)" --argjson band "$BAND" "$JQ_ROWS" <<<"$DATA" 2>/dev/null)
  extra=$(jq -r "$JQ_EXTRA" <<<"$DATA" 2>/dev/null)
  [[ -z $rows ]] && { echo "no usage windows in response (not a Pro/Max plan, or no activity yet)"; return; }

  if (( ! COMPACT )); then
    now_s=$(date '+%a %d %b %H:%M')
    lp=" claude-pace   $now_s"; lc=" ${B}claude-pace${R}   $now_s"
    if (( AGE < 0 )); then rp="from $FROM"
    elif (( STALE )); then rp="STALE $(fmt_dur "$AGE") old: $ERR"
    else rp="data $(( AGE ))s old"; fi
    if (( STALE )); then rc="$FG_on$rp$R"; else rc="$D$rp$R"; fi
    lr "$lp" "$lc" "$rp " "$rc "
    echo
  fi

  while IFS=$'\t' read -r label short ubp ebp u e d cls left budget unit flat proj hit; do
    [[ -z $label ]] && continue
    eval "fg=\$FG_$cls"
    status_word "$cls"
    (( d > 0 )) && sd="+$d" || sd="$d"

    if (( COMPACT )); then
      if [[ $cls == idle ]]; then tail="${u}% idle"; else tail="${u}%/${e}% ${sd}"; fi   # ASCII part after the bar
      w=$(( COLS - 6 - 2 - ${#tail} - 3 )); (( w < 5 )) && w=5
      printf '%-5s %s  %s%s%s %s%s%s\n' "$short" "$(bar "$w" "$ubp" "$ebp" "$cls")" \
        "$fg" "$tail" "$R" "$fg" "$GLYPH" "$R"
      continue
    fi

    if [[ $cls == idle ]]; then
      lp=" $label  ${u}% used  * $WORD"
      lc=" ${B}$label${R}  $fg$B${u}%$R used  $fg$GLYPH $WORD$R"
      rp="starts with your next message"; rc="$D$rp$R"
      lr "$lp" "$lc" "$rp " "$rc "
      w=$(( COLS - 2 )); (( w < 10 )) && w=10
      printf ' %s\n' "$(bar "$w" "$ubp" "$ebp" "$cls")"
      printf '%s   full budget available, no active window%s\n\n' "$D" "$R"
      continue
    fi
    lp=" $label  ${u}% used  pace ${e}%  (${sd})  * $WORD"
    lc=" ${B}$label${R}  $fg$B${u}%$R used  pace ${e}%  ($sd)  $fg$GLYPH $WORD$R"
    rp="resets in $(fmt_dur "$left")"; rc="$D$rp$R"
    lr "$lp" "$lc" "$rp " "$rc "

    w=$(( COLS - 2 )); (( w < 10 )) && w=10
    printf ' %s\n' "$(bar "$w" "$ubp" "$ebp" "$cls")"

    pline="   budget ${budget}%/${unit} for $(fmt_dur "$left") (flat pace ${flat}%/${unit})"
    if (( hit >= 0 && hit < left )); then
      pp="   at this rate: limit in $(fmt_dur "$hit"), $(fmt_dur $(( left - hit ))) before reset"
      if [[ $cls == ahead ]]; then pc=$FG_ahead; else pc=$FG_on; fi
    elif (( proj >= 0 )); then
      pp="   at this rate: ~${proj}% at reset"; pc=$D
    else
      pp="   too early in the window to project"; pc=$D
    fi
    if (( ${#pline} + ${#pp} <= COLS )); then
      printf '%s%s%s%s%s%s\n' "$D" "$pline" "$R" "$pc" "$pp" "$R"
    else
      printf '%s%s%s\n%s%s%s\n' "$D" "$pline" "$R" "$pc" "$pp" "$R"
    fi
    echo
  done <<<"$rows"

  [[ -n $extra ]] && printf '%s %s%s\n' "$D" "$extra" "$R"
  [[ -n $WARN ]] && printf '%s %s%s\n' "$FG_on" "$WARN" "$R"
  (( COMPACT && STALE )) && printf '%s stale data: %s%s\n' "$FG_on" "$ERR" "$R"
  return 0
}

weekly_ahead() {
  jq -r --argjson now "$(date +%s)" --argjson band "$BAND" "$JQ_ROWS" <<<"$DATA" 2>/dev/null \
    | awk -F'\t' '$2 == "7d" && $8 == "ahead" { found = 1 } END { exit !found }'
}

# ---------- main ----------
if (( RAW )); then load_data; jq . <<<"$DATA"; exit 0; fi

if (( ! WATCH )); then
  load_data
  COLS=$(term_cols); (( COLS < 60 )) && COMPACT=1
  render
  weekly_ahead && exit 2
  exit 0
fi

SLEEP_PID="" WANT_COMPACT=$COMPACT
cleanup() { [[ -n $SLEEP_PID ]] && kill "$SLEEP_PID" 2>/dev/null; printf '\033[?25h\n'; exit 0; }
trap cleanup INT TERM
trap ':' WINCH
printf '\033[?25l'
while :; do
  load_data
  COLS=$(term_cols); COMPACT=$WANT_COMPACT; (( COLS < 60 )) && COMPACT=1
  frame=$(render)
  printf '\033[H\033[2J%s\n' "$frame"
  sleep "$INTERVAL" & SLEEP_PID=$!
  wait "$SLEEP_PID" 2>/dev/null
  kill "$SLEEP_PID" 2>/dev/null; SLEEP_PID=""
done
