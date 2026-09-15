package gui

import (
	"fmt"
	"slices"
	"strconv"
)

// Preference keys. The first four predate the Settings window and keep their names.
const (
	PrefTheme   = "theme"
	PrefCompact = "compact"
	// PrefShowInDock (macOS): show pacenotch in the Dock and Cmd-Tab like a regular app,
	// instead of menu-bar-only. Useful when the camera notch hides the menu bar icon.
	PrefShowInDock = "showInDock"
	// PrefRefreshToken: when Claude Code's token has expired, run a tiny claude -p request
	// so Claude Code refreshes it.
	PrefRefreshToken = "refreshToken"
	PrefBand         = "band"
	PrefTTL          = "ttl"
	PrefNotify       = "notify"
)

// Appearance values.
const (
	ThemeDark   = "dark"
	ThemeSystem = "system"
)

// Limits of the settings.
const (
	MaxBand = 50.0
	MinTTL  = 60
	MaxTTL  = 3600
)

// Store is the part of fyne.Preferences the settings use.
type Store interface {
	BoolWithFallback(key string, fallback bool) bool
	SetBool(key string, value bool)
	FloatWithFallback(key string, fallback float64) float64
	SetFloat(key string, value float64)
	IntWithFallback(key string, fallback int) int
	SetInt(key string, value int)
	StringWithFallback(key, fallback string) string
	SetString(key, value string)
}

// Autostart turns start-at-login on and off (see internal/autostart). It is not a saved
// preference: the OS login entry is the source of truth.
type Autostart interface {
	Enabled() (bool, error)
	Enable() error
	Disable() error
}

// Settings are the GUI preferences.
type Settings struct {
	Band         float64 // "on pace" band in points
	TTL          int     // seconds cached data is used before fetching again
	Notify       bool    // notify when 7d all models goes ahead of pace
	Compact      bool
	Theme        string // ThemeDark or ThemeSystem
	ShowInDock   bool   // macOS
	RefreshToken bool
}

// DefaultSettings are used for anything not saved yet.
func DefaultSettings() Settings {
	return Settings{Band: 5, TTL: 180, Notify: true, Theme: ThemeDark, RefreshToken: true}
}

// Validate checks the ranges.
func (s Settings) Validate() error {
	switch {
	case !(s.Band > 0 && s.Band <= MaxBand):
		return fmt.Errorf("the band must be more than 0 and at most %g points", MaxBand)
	case s.TTL < MinTTL || s.TTL > MaxTTL:
		return fmt.Errorf("the refresh interval must be between %d and %d seconds", MinTTL, MaxTTL)
	case s.Theme != ThemeDark && s.Theme != ThemeSystem:
		return fmt.Errorf("the appearance must be %s or %s", ThemeDark, ThemeSystem)
	}
	return nil
}

// LoadSettings reads the saved settings; a saved value out of range falls back to its default.
func LoadSettings(st Store) Settings {
	d := DefaultSettings()
	s := Settings{
		Band:         st.FloatWithFallback(PrefBand, d.Band),
		TTL:          st.IntWithFallback(PrefTTL, d.TTL),
		Notify:       st.BoolWithFallback(PrefNotify, d.Notify),
		Compact:      st.BoolWithFallback(PrefCompact, d.Compact),
		Theme:        st.StringWithFallback(PrefTheme, d.Theme),
		ShowInDock:   st.BoolWithFallback(PrefShowInDock, d.ShowInDock),
		RefreshToken: st.BoolWithFallback(PrefRefreshToken, d.RefreshToken),
	}
	if !(s.Band > 0 && s.Band <= MaxBand) {
		s.Band = d.Band
	}
	if s.TTL < MinTTL || s.TTL > MaxTTL {
		s.TTL = d.TTL
	}
	if s.Theme != ThemeDark && s.Theme != ThemeSystem {
		s.Theme = d.Theme
	}
	return s
}

// SaveSettings writes every setting.
func SaveSettings(st Store, s Settings) {
	st.SetFloat(PrefBand, s.Band)
	st.SetInt(PrefTTL, s.TTL)
	st.SetBool(PrefNotify, s.Notify)
	st.SetBool(PrefCompact, s.Compact)
	st.SetString(PrefTheme, s.Theme)
	st.SetBool(PrefShowInDock, s.ShowInDock)
	st.SetBool(PrefRefreshToken, s.RefreshToken)
}

// Overrides are command-line flags: they apply to this session and are never saved.
type Overrides struct {
	Band    *float64
	TTL     *int
	Compact bool
}

// With applies the overrides.
func (s Settings) With(o Overrides) Settings {
	if o.Band != nil {
		s.Band = *o.Band
	}
	if o.TTL != nil {
		s.TTL = *o.TTL
	}
	if o.Compact {
		s.Compact = true
	}
	return s
}

// SettingsModel holds the saved settings and this session's overrides.
type SettingsModel struct {
	store     Store
	saved     Settings
	overrides Overrides
}

// NewSettingsModel loads the saved settings.
func NewSettingsModel(st Store, o Overrides) *SettingsModel {
	return &SettingsModel{store: st, saved: LoadSettings(st), overrides: o}
}

// Effective is what the app uses: the saved settings with the overrides applied.
func (m *SettingsModel) Effective() Settings { return m.saved.With(m.overrides) }

// Overrides returns this session's command-line overrides.
func (m *SettingsModel) Overrides() Overrides { return m.overrides }

// Update applies change to the effective settings, validates and saves the result. A field
// the change touches stops being overridden: the user's choice wins over the flag.
func (m *SettingsModel) Update(change func(*Settings)) (before, after Settings, err error) {
	before = m.Effective()
	next := before
	change(&next)
	if err := next.Validate(); err != nil {
		return before, before, err
	}
	saved, o := m.saved, m.overrides
	if next.Band != before.Band {
		saved.Band, o.Band = next.Band, nil
	}
	if next.TTL != before.TTL {
		saved.TTL, o.TTL = next.TTL, nil
	}
	if next.Compact != before.Compact {
		saved.Compact, o.Compact = next.Compact, false
	}
	saved.Notify, saved.Theme, saved.ShowInDock, saved.RefreshToken = next.Notify, next.Theme, next.ShowInDock, next.RefreshToken
	m.saved, m.overrides = saved, o
	SaveSettings(m.store, saved)
	return before, m.Effective(), nil
}

var (
	bandChoices = []float64{2, 3, 5, 8, 10, 15, 20}
	ttlChoices  = []int{60, 120, 180, 300, 600, 1800, 3600}
)

// FormatBand is the label of a band value: "5", "2.5".
func FormatBand(b float64) string { return strconv.FormatFloat(b, 'f', -1, 64) }

// BandOptions are the band choices, including the current value when it is not one of them.
func BandOptions(current float64) []string {
	values := slices.Clone(bandChoices)
	if !slices.Contains(values, current) {
		values = append(values, current)
		slices.Sort(values)
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = FormatBand(v)
	}
	return out
}

// FormatTTL is the label of a refresh interval: "3 min", "1 h", "90 s".
func FormatTTL(sec int) string {
	switch {
	case sec%3600 == 0:
		return fmt.Sprintf("%d h", sec/3600)
	case sec%60 == 0:
		return fmt.Sprintf("%d min", sec/60)
	}
	return fmt.Sprintf("%d s", sec)
}

// ParseTTL reads a FormatTTL label back.
func ParseTTL(label string) (int, bool) {
	var n int
	var unit string
	if _, err := fmt.Sscanf(label, "%d %s", &n, &unit); err != nil {
		return 0, false
	}
	switch unit {
	case "h":
		return n * 3600, true
	case "min":
		return n * 60, true
	case "s":
		return n, true
	}
	return 0, false
}

// TTLOptions are the refresh interval choices, including the current value.
func TTLOptions(current int) []string {
	values := slices.Clone(ttlChoices)
	if !slices.Contains(values, current) {
		values = append(values, current)
		slices.Sort(values)
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = FormatTTL(v)
	}
	return out
}
