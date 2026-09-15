package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Options are the command-line flags, with the reference script's defaults.
type Options struct {
	Compact  bool
	Watch    bool
	Interval int // watch redraw interval, seconds
	Band     float64
	TTL      int // cache TTL, seconds
	Width    int // -1 = detect
	From     string
	Raw      bool
	Color    string // auto, always, never
	ASCII    bool
	Help     bool
	Version  bool
	// RefreshToken lets Claude Code refresh an expired token (runs claude -p).
	RefreshToken bool
}

var (
	digitsRe = regexp.MustCompile(`^[0-9]+$`)
	numberRe = regexp.MustCompile(`^[0-9]+([.][0-9]+)?$`)
)

// ParseArgs parses flags like the reference: "-w" takes an optional number of seconds,
// value flags take the next argument or "--flag=value".
func ParseArgs(args []string, defaultTTL int) (Options, error) {
	o := Options{Interval: 60, Band: 5, TTL: defaultTTL, Width: -1, Color: "auto"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, inline, hasInline := arg, "", false
		if strings.HasPrefix(arg, "--") {
			name, inline, hasInline = strings.Cut(arg, "=")
		}
		value := func() string {
			if hasInline {
				return inline
			}
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		integer := func(msg string) (int, error) {
			v := value()
			n, err := strconv.Atoi(v)
			if !digitsRe.MatchString(v) || err != nil {
				return 0, errors.New(msg)
			}
			return n, nil
		}
		var err error
		switch name {
		case "-c", "--compact":
			o.Compact = true
		case "-w", "--watch":
			o.Watch = true
			if hasInline || (i+1 < len(args) && digitsRe.MatchString(args[i+1])) {
				o.Interval, err = integer("--watch needs seconds")
				o.Interval = max(o.Interval, 1)
			}
		case "-b", "--band":
			v := value()
			if !numberRe.MatchString(v) {
				return o, errors.New("--band needs a number")
			}
			o.Band, _ = strconv.ParseFloat(v, 64)
		case "--ttl":
			o.TTL, err = integer("--ttl needs seconds")
		case "--width":
			o.Width, err = integer("--width needs a number")
		case "--from":
			if o.From = value(); o.From == "" {
				return o, errors.New("--from needs a file")
			}
		case "--color":
			o.Color = value()
			if o.Color == "" {
				o.Color = "auto"
			}
			if o.Color != "auto" && o.Color != "always" && o.Color != "never" {
				return o, fmt.Errorf("--color must be auto, always or never, not %q", o.Color)
			}
		case "--raw", "--no-color", "--ascii", "-h", "--help", "--version", "--refresh-token":
			if hasInline {
				return o, fmt.Errorf("unknown option: %s (try --help)", arg)
			}
			switch name {
			case "--raw":
				o.Raw = true
			case "--refresh-token":
				o.RefreshToken = true
			case "--no-color":
				o.Color = "never"
			case "--ascii":
				o.ASCII = true
			case "--version":
				o.Version = true
			default:
				o.Help = true
				return o, nil
			}
		default:
			return o, fmt.Errorf("unknown option: %s (try --help)", arg)
		}
		if err != nil {
			return o, err
		}
	}
	return o, nil
}

// Help is the -h text.
const Help = `pacenotch: Claude usage limits with a vertical pace notch (unofficial, not affiliated with Anthropic)

The notch ┃ marks an even pace: fill past it means ahead of pace, fill short of it means room to spare.

  pacenotch                 full view
  pacenotch -c              compact: one line per window (automatic below 60 columns)
  pacenotch -w [SECS]       watch mode (default redraw 60s), redraws on resize
  pacenotch -b 3            "on pace" band in points (default 5)
  pacenotch --from FILE     read JSON from FILE ('-' = stdin) instead of the API;
                            accepts the API format or the status line rate_limits format
  pacenotch --raw           print the raw JSON and exit
  pacenotch gui             tray icon and window (GUI builds; pacenotch-gui.exe on Windows)
  options: --ttl SECS (cache, default 60) --width N --color auto|always|never --no-color
           --ascii (#, - and | instead of block characters) --version
           --refresh-token (or PACENOTCH_REFRESH=1): when the token has expired, run a tiny
             claude -p request so Claude Code refreshes it (pacenotch never writes it)

Exit code (single-shot mode): 0 ok, 1 error, 2 weekly (all models) is ahead of pace.
Token: $PACENOTCH_TOKEN, else Claude Code's stored credentials (read-only).
Uses the undocumented /api/oauth/usage endpoint: it may change or rate-limit (429).
`
