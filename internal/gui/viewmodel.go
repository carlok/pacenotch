package gui

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/carlok/pacenotch/internal/pace"
	"github.com/carlok/pacenotch/internal/tui"
	"github.com/carlok/pacenotch/internal/usage"
)

// Tone is how a line of text is emphasized.
type Tone int

const (
	ToneNormal Tone = iota
	ToneDim
	ToneWarn  // yellow
	ToneAlert // red
)

// RowView is one window as the GUI shows it.
type RowView struct {
	Row        pace.Row
	Title      string // "7d all models"
	Short      string // "7d"
	Compact    string // compact window line after the bar: "72%/63% +9 ▲"
	Status     string // "72% used  pace 63%  (+9)  ▲ slow down"
	Resets     string // "resets in 2d 13h"
	Budget     string // "budget 10.9%/day for 2d 13h (flat pace 14.3%/day)"
	Projection string // "at this rate: limit in 1d 17h, 20h 4m before reset"
	ProjTone   Tone
	Menu       string // tray menu line: "7d  72% / pace 63%  ▲ slow down  · resets 2d 13h"
}

// View is everything the window and the tray show for one moment.
type View struct {
	Header      string // "data 42s old", "STALE 5m old: reason", "from FILE", or the error
	HeaderTone  Tone
	Updated     string // "Fri 11 Sep 11:30"
	Error       string // set when there is nothing to show
	Stale       bool
	AuthError   bool
	NoWindows   bool
	Rows        []RowView
	Extra       string
	Warn        string
	WeeklyAhead bool
}

// IconState is the tray badge for this view.
func (v View) IconState() IconState {
	return IconState{Stale: v.Stale || v.Error != "", AuthError: v.AuthError}
}

// PaceRows returns the rows for the tray icon.
func (v View) PaceRows() []pace.Row {
	rows := make([]pace.Row, len(v.Rows))
	for i, r := range v.Rows {
		rows[i] = r.Row
	}
	return rows
}

// Loading is the view before the first load finishes.
var Loading = View{Header: "loading…", HeaderTone: ToneDim}

// BuildView turns a load result into a view at time now.
func BuildView(res usage.Result, loadErr error, now time.Time, band float64) View {
	v := View{Updated: now.Format("Mon 02 Jan 15:04"), Warn: res.Warn}
	if loadErr != nil {
		var le *usage.LoadError
		v.AuthError = errors.As(loadErr, &le) && le.Auth
		v.Error, v.Header, v.HeaderTone = loadErr.Error(), "no data: "+loadErr.Error(), ToneWarn
		return v
	}
	v.Stale, v.AuthError = res.Stale, res.Stale && res.Auth
	switch {
	case res.Age < 0:
		v.Header, v.HeaderTone = "from "+res.From, ToneDim
	case res.Stale:
		v.Header, v.HeaderTone = "STALE "+pace.Duration(res.Age)+" old: "+res.Err, ToneWarn
	default:
		v.Header, v.HeaderTone = "data "+age(res.Age)+" old", ToneDim
	}
	snap, err := usage.Parse(res.Data)
	if err != nil {
		v.Error, v.HeaderTone = err.Error(), ToneWarn
		return v
	}
	rows := pace.Rows(snap.Windows, now.Unix(), band)
	v.NoWindows, v.WeeklyAhead, v.Extra = len(rows) == 0, pace.WeeklyAhead(rows), pace.ExtraFooter(snap.Extra)
	for _, r := range rows {
		v.Rows = append(v.Rows, rowView(r))
	}
	return v
}

func age(s int64) string {
	if s < 60 {
		return strconv.FormatInt(s, 10) + "s"
	}
	return pace.Duration(s)
}

func rowView(r pace.Row) RowView {
	glyph, word := pace.Status(r.Class)
	rv := RowView{Row: r, Title: r.Label, Short: r.Short, ProjTone: ToneDim}
	if r.Class == pace.Idle {
		rv.Compact = fmt.Sprintf("%d%% idle %s", r.Used, glyph)
		rv.Status = fmt.Sprintf("%d%% used  %s %s", r.Used, glyph, word)
		rv.Resets = "starts with your next message"
		rv.Budget = "full budget available, no active window"
		rv.Menu = fmt.Sprintf("%s  %d%%  %s %s", r.Short, r.Used, glyph, word)
		return rv
	}
	sd := strconv.Itoa(r.Delta)
	if r.Delta > 0 {
		sd = "+" + sd
	}
	left := pace.Duration(r.Left)
	rv.Compact = fmt.Sprintf("%d%%/%d%% %s %s", r.Used, r.Exp, sd, glyph)
	rv.Status = fmt.Sprintf("%d%% used  pace %d%%  (%s)  %s %s", r.Used, r.Exp, sd, glyph, word)
	rv.Resets = "resets in " + left
	rv.Budget = fmt.Sprintf("budget %s%%/%s for %s (flat pace %s%%/%s)", r.Budget, r.Unit, left, r.Flat, r.Unit)
	switch {
	case r.Hit >= 0 && r.Hit < r.Left:
		rv.Projection = fmt.Sprintf("at this rate: limit in %s, %s before reset", pace.Duration(r.Hit), pace.Duration(r.Left-r.Hit))
		rv.ProjTone = ToneWarn
		if r.Class == pace.Ahead {
			rv.ProjTone = ToneAlert
		}
	case r.Proj >= 0:
		rv.Projection = fmt.Sprintf("at this rate: ~%d%% at reset", r.Proj)
	default:
		rv.Projection = "too early in the window to project"
	}
	rv.Menu = fmt.Sprintf("%s  %d%% / pace %d%%  %s %s  · resets %s", r.Short, r.Used, r.Exp, glyph, word, left)
	return rv
}

// NotifyText is the notification sent when "7d all models" goes ahead of pace.
const NotifyText = "7d all models is ahead of pace: slow down"

// AheadNotifier reports each change of "7d all models" into the ahead state, once.
// Views without data leave the state alone, so an outage does not re-trigger it.
type AheadNotifier struct {
	mu  sync.Mutex
	was bool
}

// Update returns true when v is ahead and the previous view with data was not.
func (n *AheadNotifier) Update(v View) bool {
	if v.Error != "" {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	fire := v.WeeklyAhead && !n.was
	n.was = v.WeeklyAhead
	return fire
}

// ParseArgs accepts the GUI flags: -b, -c, --ttl (default 180) and --from.
func ParseArgs(args []string) (tui.Options, error) {
	o, err := tui.ParseArgs(args, 180)
	if err != nil {
		return o, err
	}
	def, _ := tui.ParseArgs(nil, 180)
	if o.Help {
		return o, nil
	}
	if o.Watch != def.Watch || o.Interval != def.Interval || o.Width != def.Width ||
		o.Raw != def.Raw || o.Color != def.Color || o.ASCII != def.ASCII || o.Version != def.Version {
		return o, errors.New("the GUI accepts only -b, -c, --ttl and --from")
	}
	return o, nil
}

// Help is the `pacenotch gui -h` text.
const Help = `pacenotch gui: tray icon and window with the vertical pace notch

  -b N         "on pace" band in points (default 5)
  -c           compact window: one line per window (also a switch in the window and tray menu)
  --ttl SECS   cache time before fetching again (default 180)
  --from FILE  read JSON from FILE instead of the API
`
