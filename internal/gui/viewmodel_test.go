package gui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carlok/pacenotch/internal/pace"
	"github.com/carlok/pacenotch/internal/usage"
)

const testNow = 1789126200 // Fri 11 Sep 2026 11:30:00 UTC

var now = time.Unix(testNow, 0).UTC()

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBuildViewRows(t *testing.T) {
	v := BuildView(usage.Result{Data: fixture(t, "api-sample"), Age: 42}, nil, now, 5)
	if v.Header != "data 42s old" || v.HeaderTone != ToneDim || v.Updated != "Fri 11 Sep 11:30" || !v.WeeklyAhead ||
		v.Error != "" || v.IconState() != (IconState{}) || len(v.Rows) != 3 || len(v.PaceRows()) != 3 {
		t.Fatalf("view %+v", v)
	}
	want := []RowView{
		{Title: "5h session", Status: "33% used  pace 50%  (-17)  ▼ room to spare", Resets: "resets in 2h 30m",
			Budget: "budget 26.8%/h for 2h 30m (flat pace 20.0%/h)", Projection: "at this rate: ~66% at reset", ProjTone: ToneDim,
			Menu: "5h  33% / pace 50%  ▼ room to spare  · resets 2h 30m"},
		{Title: "7d all models", Status: "72% used  pace 63%  (+9)  ▲ slow down", Resets: "resets in 2d 13h",
			Budget: "budget 10.9%/day for 2d 13h (flat pace 14.3%/day)", Projection: "at this rate: limit in 1d 17h, 20h 4m before reset",
			ProjTone: ToneAlert, Menu: "7d  72% / pace 63%  ▲ slow down  · resets 2d 13h"},
		{Title: "7d Sonnet", Status: "0% used  ○ not started", Resets: "starts with your next message",
			Budget: "full budget available, no active window", ProjTone: ToneDim, Menu: "7d S  0%  ○ not started"},
	}
	for i, w := range want {
		got := v.Rows[i]
		w.Row = got.Row
		if got != w {
			t.Errorf("row %d\n got %+v\nwant %+v", i, got, w)
		}
	}
	if v.Rows[1].Row.Class != pace.Ahead || v.Rows[2].Row.Class != pace.Idle {
		t.Error("row classes")
	}
}

func TestBuildViewTones(t *testing.T) {
	v := BuildView(usage.Result{Data: fixture(t, "statusline-iso-offsets"), Age: 0}, nil, now, 5)
	if r := v.Rows[0]; r.Row.Class != pace.On || r.ProjTone != ToneWarn || r.Projection != "at this rate: limit in 2h 18m, 11m before reset" {
		t.Errorf("on pace, limit before reset: %+v", r)
	}
	v = BuildView(usage.Result{Data: fixture(t, "early"), Age: 0}, nil, now, 5)
	if r := v.Rows[1]; r.Projection != "too early in the window to project" || r.ProjTone != ToneDim {
		t.Errorf("too early: %+v", r)
	}
	if v.Extra != "extra usage: on" || v.WeeklyAhead {
		t.Errorf("extra %q", v.Extra)
	}
}

func TestBuildViewStates(t *testing.T) {
	sample := fixture(t, "api-sample")
	tests := []struct {
		name   string
		res    usage.Result
		err    error
		header string
		tone   Tone
		icon   IconState
		check  func(View) bool
	}{
		{"old data", usage.Result{Data: sample, Age: 125}, nil, "data 2m old", ToneDim, IconState{}, nil},
		{"from file", usage.Result{Data: sample, Age: -1, From: "x.json"}, nil, "from x.json", ToneDim, IconState{}, nil},
		{"stale", usage.Result{Data: sample, Age: 300, Stale: true, Err: "HTTP 429: usage endpoint is rate limiting, try again later"}, nil,
			"STALE 5m old: HTTP 429: usage endpoint is rate limiting, try again later", ToneWarn, IconState{Stale: true},
			func(v View) bool { return len(v.Rows) == 3 }},
		{"stale auth", usage.Result{Data: sample, Age: 60, Stale: true, Auth: true, Err: "HTTP 401: token rejected (start Claude Code once to refresh it)"}, nil,
			"STALE 1m old: HTTP 401: token rejected (start Claude Code once to refresh it)", ToneWarn, IconState{Stale: true, AuthError: true}, nil},
		{"auth error, no data", usage.Result{Warn: usage.WarnExpired}, &usage.LoadError{Msg: "no Claude Code credentials found (run: claude login)", Auth: true},
			"no data: no Claude Code credentials found (run: claude login)", ToneWarn, IconState{Stale: true, AuthError: true},
			func(v View) bool { return v.Warn == usage.WarnExpired && len(v.Rows) == 0 }},
		{"network error, no data", usage.Result{}, errors.New("network error"), "no data: network error", ToneWarn, IconState{Stale: true},
			func(v View) bool { return v.Error == "network error" }},
		{"invalid JSON", usage.Result{Data: []byte("{"), Age: 3}, nil, "data 3s old", ToneWarn, IconState{Stale: true},
			func(v View) bool { return v.Error == usage.ErrInvalidJSON.Error() }},
		{"no windows", usage.Result{Data: []byte("{}"), Age: 3}, nil, "data 3s old", ToneDim, IconState{},
			func(v View) bool { return v.NoWindows && len(v.Rows) == 0 }},
	}
	for _, tt := range tests {
		v := BuildView(tt.res, tt.err, now, 5)
		if v.Header != tt.header || v.HeaderTone != tt.tone || v.IconState() != tt.icon || (tt.check != nil && !tt.check(v)) {
			t.Errorf("%s: %+v", tt.name, v)
		}
	}
}

func TestAheadNotifier(t *testing.T) {
	var n AheadNotifier
	steps := []struct {
		v    View
		fire bool
	}{
		{View{WeeklyAhead: true}, true},
		{View{WeeklyAhead: true}, false},
		{View{Error: "network error"}, false},
		{View{WeeklyAhead: true}, false},
		{View{}, false},
		{View{Error: "x"}, false},
		{View{WeeklyAhead: true}, true},
	}
	for i, s := range steps {
		if got := n.Update(s.v); got != s.fire {
			t.Errorf("step %d: fire = %v", i, got)
		}
	}
}

func TestParseArgs(t *testing.T) {
	o, err := ParseArgs([]string{"-b", "3", "--ttl", "60", "--from", "x.json"})
	if err != nil || o.Band != 3 || o.TTL != 60 || o.From != "x.json" {
		t.Errorf("%+v %v", o, err)
	}
	if o, err = ParseArgs(nil); err != nil || o.TTL != 180 {
		t.Errorf("default ttl: %+v %v", o, err)
	}
	if o, err = ParseArgs([]string{"-h"}); err != nil || !o.Help {
		t.Errorf("help: %+v %v", o, err)
	}
	for _, args := range [][]string{{"-w"}, {"-c"}, {"--raw"}, {"--width", "3"}, {"--color", "never"}, {"--ascii"}, {"--version"}} {
		if _, err := ParseArgs(args); err == nil || err.Error() != "the GUI accepts only -b, --ttl and --from" {
			t.Errorf("%v: %v", args, err)
		}
	}
	if _, err := ParseArgs([]string{"--bogus"}); err == nil {
		t.Error("unknown flags are errors")
	}
}
