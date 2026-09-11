package pace

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/carlok/pacenotch/internal/usage"
)

const now = 1789126200 // Fri 11 Sep 2026 11:30:00 UTC

func active(key string, used, left float64) usage.Window {
	return usage.Window{Key: key, Used: used, Reset: now + left, HasReset: true}
}

func TestRows(t *testing.T) {
	tests := []struct {
		name string
		in   usage.Window
		band float64
		want Row
	}{
		{"5h under", active("five_hour", 33, 9000), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 3300, ExpBP: 5000, Used: 33, Exp: 50, Delta: -17, Class: Under, Left: 9000,
			Budget: "26.8", Unit: "h", Flat: "20.0", Proj: 66, Hit: -1}},
		{"7d ahead, limit before reset", active("seven_day", 72, 221399), 5, Row{
			Key: "seven_day", Label: "7d all models", Short: "7d", Window: SevenDay,
			UsedBP: 7200, ExpBP: 6339, Used: 72, Exp: 63, Delta: 9, Class: Ahead, Left: 221399,
			Budget: "10.9", Unit: "day", Flat: "14.3", Proj: 114, Hit: 149100}},
		{"delta exactly +band is on pace", active("five_hour", 55, 9000), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 5500, ExpBP: 5000, Used: 55, Exp: 50, Delta: 5, Class: On, Left: 9000,
			Budget: "18.0", Unit: "h", Flat: "20.0", Proj: 110, Hit: 7363}},
		{"delta exactly -band is on pace", active("five_hour", 45, 9000), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 4500, ExpBP: 5000, Used: 45, Exp: 50, Delta: -5, Class: On, Left: 9000,
			Budget: "22.0", Unit: "h", Flat: "20.0", Proj: 90, Hit: -1}},
		{"just past +band is ahead", active("five_hour", 55.1, 9000), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 5510, ExpBP: 5000, Used: 55, Exp: 50, Delta: 5, Class: Ahead, Left: 9000,
			Budget: "18.0", Unit: "h", Flat: "20.0", Proj: 110, Hit: 7333}},
		{"just past -band is under", active("five_hour", 44.9, 9000), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 4490, ExpBP: 5000, Used: 45, Exp: 50, Delta: -5, Class: Under, Left: 9000,
			Budget: "22.0", Unit: "h", Flat: "20.0", Proj: 90, Hit: -1}},
		{"custom band", active("five_hour", 55.1, 9000), 10, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 5510, ExpBP: 5000, Used: 55, Exp: 50, Delta: 5, Class: On, Left: 9000,
			Budget: "18.0", Unit: "h", Flat: "20.0", Proj: 110, Hit: 7333}},
		{"0% used: no projection", active("five_hour", 0, 9000), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 0, ExpBP: 5000, Used: 0, Exp: 50, Delta: -50, Class: Under, Left: 9000,
			Budget: "40.0", Unit: "h", Flat: "20.0", Proj: -1, Hit: -1}},
		{"100% used: limit already hit", active("five_hour", 100, 9000), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 10000, ExpBP: 5000, Used: 100, Exp: 50, Delta: 50, Class: Ahead, Left: 9000,
			Budget: "0.0", Unit: "h", Flat: "20.0", Proj: 200, Hit: 0}},
		{"used above 100 is clamped in the bar", active("seven_day_opus", 110, 221399), 5, Row{
			Key: "seven_day_opus", Label: "7d Opus", Short: "7d O", Window: SevenDay,
			UsedBP: 10000, ExpBP: 6339, Used: 110, Exp: 63, Delta: 47, Class: Ahead, Left: 221399,
			Budget: "0.0", Unit: "day", Flat: "14.3", Proj: 174, Hit: 0}},
		{"0% used before 5% elapsed", active("five_hour", 0, 17500), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 0, ExpBP: 278, Used: 0, Exp: 3, Delta: -3, Class: On, Left: 17500,
			Budget: "20.6", Unit: "h", Flat: "20.0", Proj: -1, Hit: -1}},
		{"elapsed exactly 5% projects", active("five_hour", 40, 17100), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 4000, ExpBP: 500, Used: 40, Exp: 5, Delta: 35, Class: Ahead, Left: 17100,
			Budget: "12.6", Unit: "h", Flat: "20.0", Proj: 800, Hit: 1350}},
		{"elapsed just under 5% is too early", active("five_hour", 40, 17101), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 4000, ExpBP: 499, Used: 40, Exp: 5, Delta: 35, Class: Ahead, Left: 17101,
			Budget: "12.6", Unit: "h", Flat: "20.0", Proj: -1, Hit: -1}},
		{"reset one second after now", active("seven_day_sonnet", 40, 1), 5, Row{
			Key: "seven_day_sonnet", Label: "7d Sonnet", Short: "7d S", Window: SevenDay,
			UsedBP: 4000, ExpBP: 10000, Used: 40, Exp: 100, Delta: -60, Class: Under, Left: 1,
			Budget: "5184000.0", Unit: "day", Flat: "14.3", Proj: 40, Hit: -1}},
		{"fractional reset", active("five_hour", 12, 9000.75), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 1200, ExpBP: 5000, Used: 12, Exp: 50, Delta: -38, Class: Under, Left: 9000,
			Budget: "35.2", Unit: "h", Flat: "20.0", Proj: 24, Hit: -1}},
		{"left larger than the window", active("seven_day_sonnet", 1, 700000), 5, Row{
			Key: "seven_day_sonnet", Label: "7d Sonnet", Short: "7d S", Window: SevenDay,
			UsedBP: 100, ExpBP: 0, Used: 1, Exp: 0, Delta: 1, Class: On, Left: SevenDay,
			Budget: "12.2", Unit: "day", Flat: "14.3", Proj: -1, Hit: -1}},
		{"reset exactly at now is idle with 0 used", active("five_hour", 88, 0), 5, Row{
			Key: "five_hour", Label: "5h session", Short: "5h", Window: FiveHour,
			UsedBP: 0, ExpBP: -1, Used: 0, Exp: -1, Delta: 0, Class: Idle, Left: -1,
			Budget: "", Unit: "h", Flat: "20.0", Proj: -1, Hit: -1}},
		{"reset one second before now is idle", active("seven_day", 95, -1), 5, Row{
			Key: "seven_day", Label: "7d all models", Short: "7d", Window: SevenDay,
			UsedBP: 0, ExpBP: -1, Used: 0, Exp: -1, Delta: 0, Class: Idle, Left: -1,
			Budget: "", Unit: "day", Flat: "14.3", Proj: -1, Hit: -1}},
		{"null reset is idle with used = utilization", usage.Window{Key: "seven_day", Used: 12.5}, 5, Row{
			Key: "seven_day", Label: "7d all models", Short: "7d", Window: SevenDay,
			UsedBP: 1250, ExpBP: -1, Used: 13, Exp: -1, Delta: 0, Class: Idle, Left: -1,
			Budget: "", Unit: "day", Flat: "14.3", Proj: -1, Hit: -1}},
		{"negative used", active("five_hour_x", -5, 9000), 5, Row{
			Key: "five_hour_x", Label: "5h x", Short: "five_", Window: FiveHour,
			UsedBP: 0, ExpBP: 5000, Used: -5, Exp: 50, Delta: -55, Class: Under, Left: 9000,
			Budget: "42.0", Unit: "h", Flat: "20.0", Proj: -1, Hit: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Rows([]usage.Window{tt.in}, now, tt.band)
			if len(got) != 1 {
				t.Fatalf("got %d rows, want 1", len(got))
			}
			if got[0] != tt.want {
				t.Errorf("\n got %+v\nwant %+v", got[0], tt.want)
			}
		})
	}
}

func TestRowsSelectionAndOrder(t *testing.T) {
	in := []usage.Window{
		active("seven_day_foo", 3, 100),
		active("seven_day", 71, 100),
		{Key: "nimbus_quill", Used: 0},
		active("five_hour_bar", 20, 100),
		{Key: "seven_day_sonnet", Used: 0},
		active("five_hour", 100, 100),
	}
	var keys []string
	for _, r := range Rows(in, now, 5) {
		keys = append(keys, r.Key)
	}
	want := []string{"five_hour", "seven_day", "seven_day_foo", "five_hour_bar", "seven_day_sonnet"}
	if len(keys) != len(want) {
		t.Fatalf("keys %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys %v, want %v", keys, want)
		}
	}
	if got := Rows(nil, now, 5); len(got) != 0 {
		t.Errorf("Rows(nil) = %v", got)
	}
}

func TestLabels(t *testing.T) {
	tests := []struct{ key, label, short string }{
		{"five_hour", "5h session", "5h"},
		{"seven_day", "7d all models", "7d"},
		{"seven_day_sonnet", "7d Sonnet", "7d S"},
		{"seven_day_opus", "7d Opus", "7d O"},
		{"seven_day_cowork", "7d cowork", "seven"},
		{"five_hour_bar", "5h bar", "five_"},
		{"seven_dayx", "seven_dayx", "seven"},
		{"xy", "xy", "xy"},
		{"seven_day_é", "7d é", "seven"},
		{"éééééé", "éééééé", "ééééé"},
	}
	for _, tt := range tests {
		if got := Label(tt.key); got != tt.label {
			t.Errorf("Label(%q) = %q, want %q", tt.key, got, tt.label)
		}
		if got := Short(tt.key); got != tt.short {
			t.Errorf("Short(%q) = %q, want %q", tt.key, got, tt.short)
		}
	}
}

func TestWindowLength(t *testing.T) {
	for key, want := range map[string]int64{"five_hour": 18000, "five_hour_x": 18000, "seven_day": 604800, "seven_dayx": 604800, "extra_usage": 0, "": 0} {
		if got := WindowLength(key); got != want {
			t.Errorf("WindowLength(%q) = %d, want %d", key, got, want)
		}
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		c           Class
		glyph, word string
	}{
		{Ahead, "▲", "slow down"}, {On, "●", "on pace"}, {Under, "▼", "room to spare"}, {Idle, "○", "not started"},
	}
	for _, tt := range tests {
		if g, w := Status(tt.c); g != tt.glyph || w != tt.word {
			t.Errorf("Status(%s) = %q %q", tt.c, g, w)
		}
	}
}

func TestWeeklyAhead(t *testing.T) {
	rows := []Row{{Short: "5h", Class: Ahead}, {Short: "7d O", Class: Ahead}, {Short: "7d", Class: On}}
	if WeeklyAhead(rows) {
		t.Error("only 5h and Opus are ahead")
	}
	rows[2].Class = Ahead
	if !WeeklyAhead(rows) {
		t.Error("7d all models is ahead")
	}
}

func TestExtraFooter(t *testing.T) {
	tests := []struct {
		in   usage.Extra
		want string
	}{
		{usage.Extra{}, ""},
		{usage.Extra{Utilization: 50, HasUtilization: true}, ""},
		{usage.Extra{Enabled: true}, "extra usage: on"},
		{usage.Extra{Enabled: true, Utilization: 37.5, HasUtilization: true}, "extra usage: on (38% of monthly limit)"},
		{usage.Extra{Enabled: true, Utilization: 37.4, HasUtilization: true}, "extra usage: on (37% of monthly limit)"},
	}
	for _, tt := range tests {
		if got := ExtraFooter(tt.in); got != tt.want {
			t.Errorf("ExtraFooter(%+v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestF1(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		// 9.95*10 rounds to exactly 99.5 in float64; jq also prints "10.0"
		{100.0 / 7, "14.3"}, {20, "20.0"}, {0, "0.0"}, {0.05, "0.1"}, {0.04, "0.0"}, {9.95, "10.0"},
		{-3, "0.0"}, {2e9, "1000000000.0"}, {26.8, "26.8"}, {math.Inf(1), "1000000000.0"},
	}
	for _, tt := range tests {
		if got := F1(tt.in); got != tt.want {
			t.Errorf("F1(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{-5, "0m"}, {0, "0m"}, {59, "0m"}, {60, "1m"}, {3599, "59m"}, {3600, "1h 0m"},
		{86399, "23h 59m"}, {86400, "1d 0h"}, {221399, "2d 13h"}, {604800, "7d 0h"},
	}
	for _, tt := range tests {
		if got := Duration(tt.in); got != tt.want {
			t.Errorf("Duration(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRoundInt(t *testing.T) {
	tests := []struct {
		in   float64
		want int
	}{
		{2.5, 3}, {-2.5, -3}, {-2.4, -2}, {1e300, 1e15}, {-1e300, -1e15}, {math.Inf(1), 1e15},
	}
	for _, tt := range tests {
		if got := roundInt(tt.in); got != tt.want {
			t.Errorf("roundInt(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// FuzzRows checks the whole pipeline from bytes to rows: it never panics, and it returns
// either an error or rows whose fields are within their documented ranges.
func FuzzRows(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "fixtures", "*.json"))
	for _, name := range files {
		if b, err := os.ReadFile(name); err == nil {
			f.Add(b, int64(now), 5.0)
		}
	}
	f.Add([]byte(`{"five_hour":{"utilization":1e308,"resets_at":-1e308}}`), int64(0), -1.0)
	f.Fuzz(func(t *testing.T, data []byte, at int64, band float64) {
		snap, err := usage.Parse(data)
		if err != nil {
			return
		}
		for _, r := range Rows(snap.Windows, at, band) {
			checkRow(t, r)
		}
	})
}

func checkRow(t *testing.T, r Row) {
	t.Helper()
	if r.Window != FiveHour && r.Window != SevenDay {
		t.Fatalf("bad window length: %+v", r)
	}
	if r.UsedBP < 0 || r.UsedBP > 10000 || r.Label == "" || r.Short == "" || (r.Unit != "h" && r.Unit != "day") {
		t.Fatalf("bad row: %+v", r)
	}
	switch r.Class {
	case Idle:
		if r.ExpBP != -1 || r.Left != -1 || r.Budget != "" || r.Proj != -1 || r.Hit != -1 {
			t.Fatalf("bad idle row: %+v", r)
		}
	case Ahead, On, Under:
		if r.ExpBP < 0 || r.ExpBP > 10000 || r.Left < 0 || r.Left > r.Window || r.Hit < -1 {
			t.Fatalf("bad active row: %+v", r)
		}
		if _, err := strconv.ParseFloat(r.Budget, 64); err != nil {
			t.Fatalf("bad budget: %+v", r)
		}
	default:
		t.Fatalf("bad class: %+v", r)
	}
}
