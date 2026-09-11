package usage

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

const reset = 1789135200 // 2026-09-11T14:00:00Z

func TestParseISO(t *testing.T) {
	ok := []struct {
		in   string
		want float64
	}{
		{"2026-09-11T14:00:00Z", reset},
		{"2026-09-11T14:00:00", reset},
		{"2026-09-11T14:00:00.528743+00:00", reset},
		{"2026-09-11T14:00:00.999999Z", reset},
		{"2026-09-11T16:00:00+02:00", reset},
		{"2026-09-11T16:00:00+0200", reset},
		{"2026-09-11T08:30:00-05:30", reset},
		{"2026-09-11T08:30:00.5-05:30", reset},
		{"2026-09-11T13:59:60Z", reset},
		{"2026-02-31T00:00:00Z", float64(time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC).Unix())},
	}
	for _, tt := range ok {
		got, err := ParseISO(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseISO(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
	bad := []string{
		"", "garbage", "2026-09-11 14:00:00Z", "2026-9-11T14:00:00Z", "2026-00-11T14:00:00Z",
		"2026-13-11T14:00:00Z", "2026-09-00T14:00:00Z", "2026-09-32T14:00:00Z",
		"2026-09-11T24:00:00Z", "2026-09-11T14:60:00Z", "2026-09-11T14:00:61Z",
		"2026-09-11T14:00:00UTC", "2026-09-11T14:00:00+2:00", "2026-09-11T14:00:00.Z",
		"2026-09-11T14:00:00+02:00:00", "2026-09-11T14:00:00\nZ",
	}
	for _, in := range bad {
		if got, err := ParseISO(in); err == nil {
			t.Errorf("ParseISO(%q) = %v, want an error", in, got)
		}
	}
}

func TestParseResetsAt(t *testing.T) {
	tests := []struct {
		raw     string
		want    float64
		ok, err bool
	}{
		{"", 0, false, false},
		{"null", 0, false, false},
		{"  null ", 0, false, false},
		{"1789347599", 1789347599, true, false},
		{"1789135200.75", 1789135200.75, true, false},
		{"-1", -1, true, false},
		{`"2026-09-11T16:00:00+02:00"`, reset, true, false},
		{`"not a date"`, 0, false, true},
		{`"\x"`, 0, false, true},
		{"1e400", 0, false, true},
		{"true", 0, false, true},
		{"{}", 0, false, true},
		{"[]", 0, false, true},
	}
	for _, tt := range tests {
		got, ok, err := ParseResetsAt(json.RawMessage(tt.raw))
		if got != tt.want || ok != tt.ok || (err != nil) != tt.err {
			t.Errorf("ParseResetsAt(%s) = %v, %v, %v; want %v, %v, err=%v", tt.raw, got, ok, err, tt.want, tt.ok, tt.err)
		}
	}
}

// FuzzParseResetsAt: never panics; an error means no value; a value is finite.
func FuzzParseResetsAt(f *testing.F) {
	for _, s := range []string{`"2026-09-11T14:00:00.528743+00:00"`, `"2026-09-13T19:29:59-05:30"`, "1789347599", "null", `"2026-09-11T16:00:00+0200"`, "1.5e9"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		v, ok, err := ParseResetsAt(json.RawMessage(raw))
		if err != nil && ok {
			t.Fatalf("error with a value: %q", raw)
		}
		if ok && (math.IsNaN(v) || math.IsInf(v, 0)) {
			t.Fatalf("non-finite value %v for %q", v, raw)
		}
		if len(raw) > 0 && raw[0] == '"' {
			var s string
			if json.Unmarshal([]byte(raw), &s) == nil {
				if iso, err := ParseISO(s); (err == nil) != ok || (ok && iso != v) {
					t.Fatalf("string and ISO parse disagree for %q", raw)
				}
			}
		}
	})
}
