//go:build gui

package gui

import (
	"runtime"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Settings form labels.
const (
	LabelBand         = "On pace band"
	LabelTTL          = "Refresh data every"
	LabelNotify       = "Notifications"
	LabelCompact      = "Window"
	LabelTheme        = "Appearance"
	LabelDock         = "Dock"
	LabelRefreshToken = "Expired token"
	LabelLogin        = "Login"
)

// ShowSettings opens the Settings window. Changes apply and save right away.
func (u *UI) ShowSettings() {
	if u.settingsWin == nil {
		u.settingsWin = u.App.NewWindow("pacenotch settings")
		u.settingsWin.SetCloseIntercept(u.settingsWin.Hide)
		u.settingsWin.Resize(fyne.NewSize(500, 480))
	}
	u.settingsWin.SetContent(u.settingsContent())
	u.settingsWin.Show()
	u.settingsWin.RequestFocus()
	bringToFront()
}

func (u *UI) settingsContent() fyne.CanvasObject {
	s := u.Settings.Effective()
	ov := u.Settings.Overrides()

	// every widget gets its value first and its callback after, so building the form does
	// not trigger updates
	band := widget.NewSelect(BandOptions(s.Band), nil)
	band.SetSelected(FormatBand(s.Band))
	band.OnChanged = func(v string) {
		if b, err := strconv.ParseFloat(v, 64); err == nil {
			u.Update(func(s *Settings) { s.Band = b })
		}
	}
	ttl := widget.NewSelect(TTLOptions(s.TTL), nil)
	ttl.SetSelected(FormatTTL(s.TTL))
	ttl.OnChanged = func(v string) {
		if n, ok := ParseTTL(v); ok {
			u.Update(func(s *Settings) { s.TTL = n })
		}
	}
	check := func(text string, on bool, set func(*Settings, bool)) *widget.Check {
		c := widget.NewCheck(text, nil)
		c.SetChecked(on)
		c.OnChanged = func(v bool) { u.Update(func(s *Settings) { set(s, v) }) }
		return c
	}
	theme := widget.NewSelect([]string{"Dark", "System"}, nil)
	theme.SetSelected(map[string]string{ThemeDark: "Dark", ThemeSystem: "System"}[s.Theme])
	theme.OnChanged = func(v string) {
		u.Update(func(s *Settings) { s.Theme = map[string]string{"Dark": ThemeDark, "System": ThemeSystem}[v] })
	}

	bandHint := "points around the notch that still count as on pace"
	if ov.Band != nil {
		bandHint = "set by -b for this session; choosing a value saves it"
	}
	ttlHint := "how long data is reused before asking the endpoint again"
	if ov.TTL != nil {
		ttlHint = "set by --ttl for this session; choosing a value saves it"
	}
	form := widget.NewForm(
		&widget.FormItem{Text: LabelBand, Widget: band, HintText: bandHint},
		&widget.FormItem{Text: LabelTTL, Widget: ttl, HintText: ttlHint},
		&widget.FormItem{Text: LabelNotify, Widget: check("When 7d all models goes ahead of pace", s.Notify, func(s *Settings, v bool) { s.Notify = v })},
		&widget.FormItem{Text: LabelCompact, Widget: check("Compact: one line per window", s.Compact, func(s *Settings, v bool) { s.Compact = v })},
		&widget.FormItem{Text: LabelTheme, Widget: theme},
	)
	if runtime.GOOS == "darwin" {
		form.Append(LabelDock, check("Show in Dock and Cmd-Tab", s.ShowInDock, func(s *Settings, v bool) { s.ShowInDock = v }))
	}
	if u.Autostart != nil {
		on, err := u.Autostart.Enabled()
		login := widget.NewCheck("Start pacenotch when I log in", nil)
		login.SetChecked(on)
		login.OnChanged = func(v bool) {
			if v {
				u.loginErr = u.Autostart.Enable()
			} else {
				u.loginErr = u.Autostart.Disable()
			}
			u.settingsWin.SetContent(u.settingsContent())
		}
		hint := "adds a login item for this copy of pacenotch"
		switch {
		case u.loginErr != nil:
			hint = "could not change the login item: " + u.loginErr.Error()
		case err != nil:
			hint = "cannot read the login item: " + err.Error()
		}
		form.AppendItem(&widget.FormItem{Text: LabelLogin, Widget: login, HintText: hint})
	}
	form.AppendItem(&widget.FormItem{
		Text:     LabelRefreshToken,
		Widget:   check("Refresh it through Claude Code", s.RefreshToken, func(s *Settings, v bool) { s.RefreshToken = v }),
		HintText: "runs a tiny claude -p request; pacenotch never writes the token",
	})
	return container.NewPadded(container.NewVScroll(form))
}
