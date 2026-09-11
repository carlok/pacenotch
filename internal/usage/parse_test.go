package usage

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseFixtures(t *testing.T) {
	tests := []struct {
		file    string
		windows []Window
		extra   Extra
	}{
		{"fixtures/api-sample.json", []Window{
			{"five_hour", 33, reset, true},
			{"seven_day", 72, 1789347599, true},
			{"seven_day_sonnet", 0, 0, false},
		}, Extra{}},
		{"fixtures/api-real-shape.json", []Window{
			{"five_hour", 10, reset - 600, true},
			{"seven_day", 47, 1789434000, true},
			{"nimbus_quill", 0, 0, false},
			{"extra_usage", 0, 0, false}, // numeric utilization: a candidate, pace drops it
		}, Extra{}},
		{"fixtures/statusline-epoch.json", []Window{
			{"five_hour", 12, 1789135200.75, true},
			{"seven_day", 64, 1789347599, true},
		}, Extra{}},
		{"fixtures/statusline-iso-offsets.json", []Window{
			{"five_hour", 52, reset + 7200 - 7200, true},
			{"seven_day", 30, 1789347599, true},
			{"seven_day_sonnet", 5, 1789347599, true},
			{"seven_day_opus", 70, 1789347599, true},
		}, Extra{}},
		{"fixtures/extra-enabled.json", []Window{
			{"seven_day_foo", 3, 1789347599, true},
			{"seven_day", 71, 1789347599, true},
			{"five_hour_bar", 20, reset, true},
			{"seven_day_opus", 110, 1789347599, true},
			{"five_hour", 100, reset, true},
			{"extra_usage", 37.6, 0, false},
		}, Extra{Enabled: true, Utilization: 37.6, HasUtilization: true}},
		{"fixtures/early.json", []Window{
			{"five_hour", 40, 1789143300, true},
			{"seven_day", 2, 1789700761, true},
			{"seven_day_sonnet", 1, 1789826200, true},
		}, Extra{Enabled: true}},
		{"fixtures/idle.json", []Window{
			{"five_hour", 0, 0, false},
			{"seven_day", 12.5, 0, false},
		}, Extra{}},
		{"fixtures/empty.json", nil, Extra{}},
		{"robust/wrong-types.json", nil, Extra{}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			snap, err := Parse(fixture(t, tt.file))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(snap.Windows, tt.windows) {
				t.Errorf("windows\n got %+v\nwant %+v", snap.Windows, tt.windows)
			}
			if snap.Extra != tt.extra {
				t.Errorf("extra %+v, want %+v", snap.Extra, tt.extra)
			}
		})
	}
}

func TestParseShapes(t *testing.T) {
	tests := []struct {
		name, in string
		windows  []Window
		extra    Extra
	}{
		{"top-level array", `[1,2,3]`, nil, Extra{}},
		{"top-level number", `42`, nil, Extra{}},
		{"rate_limits null hides top-level windows", `{"rate_limits":null,"five_hour":{"utilization":1}}`, nil, Extra{}},
		{"rate_limits wins", `{"five_hour":{"utilization":1},"rate_limits":{"seven_day":{"used_percentage":2}}}`,
			[]Window{{Key: "seven_day", Used: 2}}, Extra{}},
		{"utilization null falls back to used_percentage", `{"five_hour":{"utilization":null,"used_percentage":7}}`,
			[]Window{{Key: "five_hour", Used: 7}}, Extra{}},
		{"utilization false falls back", `{"five_hour":{"utilization":false,"used_percentage":8}}`,
			[]Window{{Key: "five_hour", Used: 8}}, Extra{}},
		{"utilization string is skipped", `{"five_hour":{"utilization":"33","used_percentage":8}}`, nil, Extra{}},
		{"no percentage is skipped", `{"five_hour":{"utilization":null,"used_percentage":null}}`, nil, Extra{}},
		{"used_percentage string is skipped", `{"five_hour":{"used_percentage":"x"}}`, nil, Extra{}},
		{"zero is kept", `{"five_hour":{"utilization":0}}`, []Window{{Key: "five_hour"}}, Extra{}},
		{"bad resets_at is skipped", `{"five_hour":{"utilization":1,"resets_at":"soon"},"seven_day":{"utilization":2}}`,
			[]Window{{Key: "seven_day", Used: 2}}, Extra{}},
		{"unknown keys are kept for pace to filter", `{"x":{"utilization":1,"extra":[1,{"a":null}]}}`,
			[]Window{{Key: "x", Used: 1}}, Extra{}},
		{"repeated key keeps first position and last value",
			`{"five_hour":{"utilization":1},"seven_day":{"utilization":2},"five_hour":{"utilization":3}}`,
			[]Window{{Key: "five_hour", Used: 3}, {Key: "seven_day", Used: 2}}, Extra{}},
		{"extra on without utilization", `{"extra_usage":{"is_enabled":true}}`, nil, Extra{Enabled: true}},
		{"extra utilization string: no footer", `{"extra_usage":{"is_enabled":true,"utilization":"37"}}`, nil, Extra{}},
		{"extra is_enabled string", `{"extra_usage":{"is_enabled":"true","utilization":3}}`,
			[]Window{{Key: "extra_usage", Used: 3}}, Extra{}},
		{"utilization out of float64 range is skipped", `{"five_hour":{"utilization":1e400}}`, nil, Extra{}},
		{"extra not an object", `{"extra_usage":"yes"}`, nil, Extra{}},
		{"extra utilization false", `{"extra_usage":{"is_enabled":true,"utilization":false}}`, nil, Extra{Enabled: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap, err := Parse([]byte(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(snap.Windows, tt.windows) {
				t.Errorf("windows\n got %+v\nwant %+v", snap.Windows, tt.windows)
			}
			if snap.Extra != tt.extra {
				t.Errorf("extra %+v, want %+v", snap.Extra, tt.extra)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	for _, in := range []string{"", "{", `{"five_hour":`, "{} {}", "nul", string(fixture(t, "robust/truncated.json"))} {
		if _, err := Parse([]byte(in)); !errors.Is(err, ErrInvalidJSON) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalidJSON", in, err)
		}
	}
}

func TestIsObject(t *testing.T) {
	tests := map[string]bool{
		`{}`: true, ` {"a":[1]} `: true, `[]`: false, `{`: false, `{"a":}`: false, `{"a":1`: false,
		`{,}`: false, `{1:2}`: false, `{"a":1} x`: false, `{"a":1}{}`: false, ``: false, `{"a":1]`: false,
	}
	for in, want := range tests {
		if got := IsObject([]byte(in)); got != want {
			t.Errorf("IsObject(%q) = %v, want %v", in, got, want)
		}
	}
}

// FuzzParse: never panics; an error only for invalid JSON; parsed values are finite.
func FuzzParse(f *testing.F) {
	for _, name := range []string{"api-sample", "api-real-shape", "statusline-epoch", "statusline-iso-offsets", "extra-enabled", "idle", "early"} {
		f.Add(fixture(f, "fixtures/"+name+".json"))
	}
	f.Add(fixture(f, "robust/wrong-types.json"))
	f.Add(fixture(f, "robust/truncated.json"))
	f.Fuzz(func(t *testing.T, data []byte) {
		snap, err := Parse(data)
		if err != nil {
			if json.Valid(data) {
				t.Fatalf("error %v for valid JSON", err)
			}
			return
		}
		for _, w := range snap.Windows {
			if math.IsNaN(w.Used) || math.IsInf(w.Used, 0) || (w.HasReset && (math.IsNaN(w.Reset) || math.IsInf(w.Reset, 0))) {
				t.Fatalf("non-finite window %+v", w)
			}
		}
		if snap.Extra.HasUtilization && !snap.Extra.Enabled {
			t.Fatalf("utilization without enabled: %+v", snap.Extra)
		}
	})
}
