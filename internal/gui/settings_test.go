package gui

import (
	"reflect"
	"strings"
	"testing"
)

// mapStore is an in-memory Store.
type mapStore map[string]any

func (m mapStore) BoolWithFallback(k string, f bool) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return f
}
func (m mapStore) SetBool(k string, v bool) { m[k] = v }
func (m mapStore) FloatWithFallback(k string, f float64) float64 {
	if v, ok := m[k].(float64); ok {
		return v
	}
	return f
}
func (m mapStore) SetFloat(k string, v float64) { m[k] = v }
func (m mapStore) IntWithFallback(k string, f int) int {
	if v, ok := m[k].(int); ok {
		return v
	}
	return f
}
func (m mapStore) SetInt(k string, v int) { m[k] = v }
func (m mapStore) StringWithFallback(k, f string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return f
}
func (m mapStore) SetString(k, v string) { m[k] = v }

func TestLoadAndSaveSettings(t *testing.T) {
	if got := LoadSettings(mapStore{}); got != DefaultSettings() {
		t.Errorf("empty store: %+v", got)
	}
	want := Settings{Band: 2.5, TTL: 600, Notify: false, Compact: true, Theme: ThemeSystem, ShowInDock: true, RefreshToken: false, CheckUpdates: true}
	st := mapStore{}
	SaveSettings(st, want)
	if got := LoadSettings(st); got != want {
		t.Errorf("round trip %+v, want %+v", got, want)
	}
	bad := mapStore{PrefBand: -1.0, PrefTTL: 5, PrefTheme: "pink"}
	if got := LoadSettings(bad); got != DefaultSettings() {
		t.Errorf("out-of-range values must fall back: %+v", got)
	}
	if got := LoadSettings(mapStore{PrefBand: 51.0, PrefTTL: 3601}); got.Band != 5 || got.TTL != 180 {
		t.Errorf("above the maximum: %+v", got)
	}
}

func TestValidateSettings(t *testing.T) {
	ok := DefaultSettings()
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Settings){
		func(s *Settings) { s.Band = 0 },
		func(s *Settings) { s.Band = 50.5 },
		func(s *Settings) { s.TTL = 59 },
		func(s *Settings) { s.TTL = 3601 },
		func(s *Settings) { s.Theme = "" },
	} {
		s := ok
		change(&s)
		if err := s.Validate(); err == nil {
			t.Errorf("%+v must be invalid", s)
		}
	}
	s := ok
	s.Band, s.TTL = MaxBand, MaxTTL
	if err := s.Validate(); err != nil {
		t.Errorf("the maximums are valid: %v", err)
	}
}

func TestSettingsModel(t *testing.T) {
	band, ttl := 3.0, 60
	st := mapStore{PrefTheme: ThemeSystem}
	m := NewSettingsModel(st, Overrides{Band: &band, TTL: &ttl, Compact: true})
	eff := m.Effective()
	if eff.Band != 3 || eff.TTL != 60 || !eff.Compact || eff.Theme != ThemeSystem || m.Overrides().Band == nil {
		t.Fatalf("effective %+v", eff)
	}

	// changing another field keeps the overrides and does not save them
	before, after, err := m.Update(func(s *Settings) { s.Notify = false })
	if err != nil || !before.Notify || after.Notify || after.Band != 3 || st[PrefBand] != 5.0 || st[PrefNotify] != false {
		t.Fatalf("notify: %+v -> %+v, %v, store %v", before, after, err, st)
	}

	// changing an overridden field saves it and drops that override only
	_, after, _ = m.Update(func(s *Settings) { s.Band = 8 })
	if after.Band != 8 || st[PrefBand] != 8.0 || m.Overrides().Band != nil || m.Overrides().TTL == nil {
		t.Fatalf("band: %+v, store %v, overrides %+v", after, st, m.Overrides())
	}
	_, after, _ = m.Update(func(s *Settings) { s.TTL, s.Compact = 300, false })
	if after.TTL != 300 || after.Compact || st[PrefTTL] != 300 || st[PrefCompact] != false || m.Overrides() != (Overrides{}) {
		t.Fatalf("ttl and compact: %+v, store %v", after, st)
	}

	// invalid changes are refused and nothing moves
	before, after, err = m.Update(func(s *Settings) { s.TTL = 1 })
	if err == nil || before != after || st[PrefTTL] != 300 {
		t.Fatalf("invalid: %v %+v", err, after)
	}
}

func TestWith(t *testing.T) {
	b, ttl := 2.0, 120
	s := DefaultSettings().With(Overrides{Band: &b, TTL: &ttl, Compact: true})
	if s.Band != 2 || s.TTL != 120 || !s.Compact {
		t.Errorf("%+v", s)
	}
	if DefaultSettings().With(Overrides{}) != DefaultSettings() {
		t.Error("no overrides, no change")
	}
}

func TestChoices(t *testing.T) {
	if got := BandOptions(5); !reflect.DeepEqual(got, []string{"2", "3", "5", "8", "10", "15", "20"}) {
		t.Errorf("band options %q", got)
	}
	if got := BandOptions(2.5); strings.Join(got, ",") != "2,2.5,3,5,8,10,15,20" {
		t.Errorf("custom band %q", got)
	}
	if got := TTLOptions(180); strings.Join(got, ",") != "1 min,2 min,3 min,5 min,10 min,30 min,1 h" {
		t.Errorf("ttl options %q", got)
	}
	if got := TTLOptions(90); strings.Join(got, ",") != "1 min,90 s,2 min,3 min,5 min,10 min,30 min,1 h" {
		t.Errorf("custom ttl %q", got)
	}
	for label, want := range map[string]int{"1 min": 60, "90 s": 90, "1 h": 3600, "30 min": 1800} {
		if got, ok := ParseTTL(label); !ok || got != want || FormatTTL(want) != label {
			t.Errorf("ParseTTL(%q) = %d, %v", label, got, ok)
		}
	}
	for _, bad := range []string{"", "soon", "5 days"} {
		if _, ok := ParseTTL(bad); ok {
			t.Errorf("ParseTTL(%q) must fail", bad)
		}
	}
}
