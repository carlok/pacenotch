//go:build gui

package gui

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/carlok/pacenotch/internal/update"
	"github.com/carlok/pacenotch/internal/usage"
)

// texts collects every piece of text in an object tree.
func texts(o fyne.CanvasObject) []string {
	switch x := o.(type) {
	case *canvas.Text:
		return []string{x.Text}
	case *widget.Label:
		return []string{x.Text}
	case *widget.Button:
		return []string{x.Text}
	case *widget.Hyperlink:
		return []string{x.Text}
	case *widget.Check:
		return []string{x.Text}
	case *widget.Select:
		return []string{x.Selected}
	case *widget.Form:
		var out []string
		for _, it := range x.Items {
			out = append(out, it.Text, it.HintText)
			out = append(out, texts(it.Widget)...)
		}
		return out
	case *container.Scroll:
		return texts(x.Content)
	case *fyne.Container:
		var out []string
		for _, c := range x.Objects {
			out = append(out, texts(c)...)
		}
		return out
	}
	return nil
}

func findButton(o fyne.CanvasObject, label string) *widget.Button {
	switch x := o.(type) {
	case *widget.Button:
		if x.Text == label {
			return x
		}
	case *container.Scroll:
		return findButton(x.Content, label)
	case *fyne.Container:
		for _, c := range x.Objects {
			if b := findButton(c, label); b != nil {
				return b
			}
		}
	}
	return nil
}

func findIconButton(o fyne.CanvasObject, icon string) *widget.Button {
	switch x := o.(type) {
	case *widget.Button:
		if x.Text == "" && x.Icon != nil && x.Icon.Name() == icon {
			return x
		}
	case *container.Scroll:
		return findIconButton(x.Content, icon)
	case *fyne.Container:
		for _, c := range x.Objects {
			if b := findIconButton(c, icon); b != nil {
				return b
			}
		}
	}
	return nil
}

// formWidget returns the widget of the form item with the given label.
func formWidget(o fyne.CanvasObject, label string) fyne.CanvasObject {
	switch x := o.(type) {
	case *widget.Form:
		for _, it := range x.Items {
			if it.Text == label {
				return it.Widget
			}
		}
	case *container.Scroll:
		return formWidget(x.Content, label)
	case *fyne.Container:
		for _, c := range x.Objects {
			if w := formWidget(c, label); w != nil {
				return w
			}
		}
	}
	return nil
}

func sampleView(t *testing.T) View {
	return BuildView(usage.Result{Data: fixture(t, "api-sample"), Age: 12}, nil, now, 5)
}

func TestWindowBuilds(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	u.Start()
	u.Show(sampleView(t))
	all := strings.Join(texts(u.Window.Content()), "\n")
	for _, want := range []string{"pacenotch", "data 12s old", "7d all models", "72% used  pace 63%  (+9)  ▲ slow down",
		"resets in 2d 13h", "at this rate: limit in 1d 17h, 20h 4m before reset", "Refresh now", "Settings", "About", "Quit"} {
		if !strings.Contains(all, want) {
			t.Errorf("window is missing %q in\n%s", want, all)
		}
	}
	bar := Bar(sampleView(t).Rows[1].Row, DarkBars())
	if img := bar.Generator(300, 24); img.Bounds().Dx() != 300 {
		t.Error("the bar raster must stretch to the requested width")
	}

	u.Show(BuildView(usage.Result{}, &usage.LoadError{Msg: "no Claude Code credentials found (run: claude login)", Auth: true}, now, 5))
	if all = strings.Join(texts(u.Window.Content()), "\n"); !strings.Contains(all, "no data: no Claude Code credentials found") {
		t.Errorf("error view:\n%s", all)
	}

	u.Window.Close() // intercepted: hidden, the app keeps running
	u.ShowWindow()
	if runtime.GOOS == "darwin" {
		var labels []string
		for _, m := range u.Window.MainMenu().Items {
			for _, it := range m.Items {
				labels = append(labels, it.Label)
			}
		}
		if !strings.Contains(strings.Join(labels, ","), "Settings…") {
			t.Errorf("the macOS main menu needs Settings…: %v", labels)
		}
	}
}

func TestTrayMenu(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	u.Show(BuildView(usage.Result{Data: fixture(t, "api-sample"), Age: 400, Stale: true, Err: "network error"}, nil, now, 5))
	var labels []string
	for _, it := range u.Menu().Items {
		labels = append(labels, it.Label)
	}
	all := strings.Join(labels, "\n")
	for _, want := range []string{"⚠ STALE 6m old: network error", "7d  72% / pace 63%  ▲ slow down  · resets 2d 13h",
		"5h  33% / pace 50%  ▼ room to spare  · resets 2h 30m", "Open window", "Refresh now", "Compact window", "Settings…", "About pacenotch", "Quit"} {
		if !strings.Contains(all, want) {
			t.Errorf("menu is missing %q in\n%s", want, all)
		}
	}
	u.Show(BuildView(usage.Result{}, &usage.LoadError{Msg: "network error"}, now, 5))
	if first := u.Menu().Items[0].Label; first != "⚠ network error" {
		t.Errorf("error line %q", first)
	}
	if res := u.TrayResource(); len(res.Content()) == 0 {
		t.Error("empty tray icon")
	}
}

func TestRefreshCallsTheFetcher(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	var loads atomic.Int32
	ctrl := &Controller{
		Load: func(context.Context, bool) (usage.Result, error) {
			loads.Add(1)
			return usage.Result{Data: fixture(t, "api-sample")}, nil
		},
		Now: func() time.Time { return now }, Band: 5, Publish: u.Show,
	}
	u.OnRefresh = func() { ctrl.Refresh(context.Background(), true) }
	u.Start()
	test.Tap(findButton(u.Window.Content(), "Refresh now"))
	for _, it := range u.Menu().Items {
		if it.Label == "Refresh now" {
			it.Action()
		}
	}
	if loads.Load() != 2 {
		t.Errorf("loads = %d, want 2", loads.Load())
	}
}

func TestThemeChoice(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	if u.ThemeChoice() != ThemeDark || !u.Dark() {
		t.Fatal("dark is the default")
	}
	u.SetThemeChoice(ThemeSystem)
	u.SetThemeChoice(ThemeSystem) // no-op
	if a.Preferences().String(PrefTheme) != ThemeSystem || u.ThemeChoice() != ThemeSystem {
		t.Error("the choice must be saved")
	}
	u.SetThemeChoice("pink")
	if u.ThemeChoice() != ThemeSystem {
		t.Error("invalid values are refused")
	}
}

func TestCompactWindow(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	u.Start()
	u.Show(sampleView(t))
	if u.Compact() {
		t.Fatal("compact is off by default")
	}
	u.SetCompact(true)
	u.SetCompact(true) // no-op
	all := strings.Join(texts(u.Window.Content()), "\n")
	for _, want := range []string{"data 12s old", "5h", "33%/50% -17 ▼", "7d", "72%/63% +9 ▲", "7d S", "0% idle ○"} {
		if !strings.Contains(all, want) {
			t.Errorf("compact window is missing %q in\n%s", want, all)
		}
	}
	if strings.Contains(all, "budget") || strings.Contains(all, "resets in") {
		t.Errorf("compact window must not show the detail lines:\n%s", all)
	}
	if !a.Preferences().Bool(PrefCompact) || u.Window.Canvas().Size().Width > windowSize(false).Width {
		t.Error("the choice is saved and the window shrinks")
	}
	for _, it := range u.Menu().Items {
		if it.Label == "Compact window" {
			if !it.Checked {
				t.Error("the menu item must be checked")
			}
			it.Action()
		}
	}
	if u.Compact() || !strings.Contains(strings.Join(texts(u.Window.Content()), "\n"), "budget") {
		t.Error("the menu item must switch back to the full view")
	}
}

func TestQuitAndDockMode(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	quits := 0
	u.Quit = func() { quits++ }
	u.Start()
	u.Show(sampleView(t))

	want := 1
	test.Tap(findButton(u.Window.Content(), "Quit"))
	if sc, ok := u.Window.Canvas().(interface{ TypedShortcut(fyne.Shortcut) }); ok {
		sc.TypedShortcut(QuitShortcut) // the test canvas has no shortcut dispatch; the real one does
		want++
	}
	u.SetCompact(true)
	quitIcon := findIconButton(u.Window.Content(), theme.CancelIcon().Name())
	if quitIcon == nil || findIconButton(u.Window.Content(), theme.SettingsIcon().Name()) == nil {
		t.Fatal("the compact footer needs icon-only Settings and Quit buttons")
	}
	test.Tap(quitIcon)
	if want++; quits != want {
		t.Errorf("quit called %d times, want %d", quits, want)
	}

	if u.ShowInDock() {
		t.Fatal("Dock mode is off by default")
	}
	u.SetShowInDock(true)
	if !u.ShowInDock() || !a.Preferences().Bool(PrefShowInDock) {
		t.Error("the Dock mode choice must be saved")
	}
}

func TestSettingsWindow(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	var changes [][2]Settings
	u.OnSettings = func(before, after Settings) { changes = append(changes, [2]Settings{before, after}) }
	u.Start()
	u.ShowSettings()
	u.ShowSettings() // reuses the window
	content := u.settingsWin.Content()
	all := strings.Join(texts(content), "\n")
	for _, want := range []string{LabelBand, "5", LabelTTL, "3 min", LabelNotify, LabelTheme, "Dark", LabelRefreshToken, "never writes the token"} {
		if !strings.Contains(all, want) {
			t.Errorf("settings are missing %q in\n%s", want, all)
		}
	}
	if (runtime.GOOS == "darwin") != strings.Contains(all, "Show in Dock and Cmd-Tab") {
		t.Error("the Dock setting is macOS only")
	}

	formWidget(content, LabelBand).(*widget.Select).SetSelected("10")
	content = u.settingsWin.Content() // rebuilt after the change
	formWidget(content, LabelTTL).(*widget.Select).SetSelected("5 min")
	content = u.settingsWin.Content()
	test.Tap(formWidget(content, LabelNotify).(*widget.Check))
	content = u.settingsWin.Content()
	formWidget(content, LabelTheme).(*widget.Select).SetSelected("System")
	content = u.settingsWin.Content()
	test.Tap(formWidget(content, LabelRefreshToken).(*widget.Check))

	s := u.Settings.Effective()
	if s.Band != 10 || s.TTL != 300 || s.Notify || s.Theme != ThemeSystem || s.RefreshToken {
		t.Errorf("settings %+v", s)
	}
	if a.Preferences().Float(PrefBand) != 10 || a.Preferences().Int(PrefTTL) != 300 || len(changes) != 5 ||
		changes[0][0].Band != 5 || changes[0][1].Band != 10 {
		t.Errorf("saved band %v ttl %v, changes %+v", a.Preferences().Float(PrefBand), a.Preferences().Int(PrefTTL), changes)
	}

	band := 3.0
	u.ApplyOverrides(Overrides{Band: &band})
	u.ShowSettings()
	if all = strings.Join(texts(u.settingsWin.Content()), "\n"); !strings.Contains(all, "set by -b for this session") || u.Settings.Effective().Band != 3 {
		t.Errorf("override hint missing:\n%s", all)
	}
}

type fakeAutostart struct {
	on    bool
	err   error
	calls int
}

func (f *fakeAutostart) Enabled() (bool, error) { return f.on, nil }

func (f *fakeAutostart) Enable() error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	f.on = true
	return nil
}

func (f *fakeAutostart) Disable() error {
	f.calls++
	f.on = false
	return nil
}

func TestLoginItemSetting(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	u.ShowSettings()
	if formWidget(u.settingsWin.Content(), LabelLogin) != nil {
		t.Error("no login setting without autostart support")
	}
	fa := &fakeAutostart{}
	u.Autostart = fa
	u.ShowSettings()
	test.Tap(formWidget(u.settingsWin.Content(), LabelLogin).(*widget.Check))
	if !fa.on || fa.calls != 1 {
		t.Fatalf("enable: %+v", fa)
	}
	test.Tap(formWidget(u.settingsWin.Content(), LabelLogin).(*widget.Check))
	fa.err = errors.New("permission denied")
	test.Tap(formWidget(u.settingsWin.Content(), LabelLogin).(*widget.Check))
	all := strings.Join(texts(u.settingsWin.Content()), "\n")
	if fa.on || fa.calls != 3 || !strings.Contains(all, "could not change the login item: permission denied") {
		t.Errorf("failed enable: %+v\n%s", fa, all)
	}
}

func TestUpdateNotice(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "v0.3.0")
	u.Show(sampleView(t))
	u.ShowSettings()
	test.Tap(formWidget(u.settingsWin.Content(), LabelUpdates).(*widget.Check))
	if !u.Settings.Effective().CheckUpdates || !a.Preferences().Bool(PrefCheckUpdates) {
		t.Error("the update check setting must be saved")
	}

	rel := update.Release{Tag: "v0.4.0", URL: update.ReleasesPrefix + "tag/v0.4.0"}
	u.SetUpdate(rel)
	var item *fyne.MenuItem
	for _, it := range u.Menu().Items {
		if it.Label == "Update available: v0.4.0" {
			item = it
		}
	}
	if item == nil {
		t.Fatal("the tray menu needs the update notice")
	}
	item.Action() // opens the release page through the (test) app
	u.ShowAbout()
	if all := strings.Join(texts(u.about.Content()), "\n"); !strings.Contains(all, "Update available: v0.4.0") {
		t.Errorf("About is missing the notice:\n%s", all)
	}
	u.SetUpdate(update.Release{})
	for _, it := range u.Menu().Items {
		if strings.HasPrefix(it.Label, "Update available") {
			t.Error("the notice must go away")
		}
	}
}

func TestRefreshTokenSetting(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	var got []bool
	u.OnSettings = func(before, after Settings) { got = append(got, after.RefreshToken) }
	if !u.RefreshToken() {
		t.Fatal("token refresh is on by default")
	}
	u.SetRefreshToken(true) // no-op
	u.SetRefreshToken(false)
	if u.RefreshToken() || a.Preferences().BoolWithFallback(PrefRefreshToken, true) || len(got) != 1 || got[0] {
		t.Errorf("turning it off must save and notify: %v", got)
	}
}

func TestAboutWindow(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "v9")
	u.ShowAbout()
	u.ShowAbout()                                      // reuses the window
	all := strings.Join(texts(u.about.Content()), " ") // wrapped paragraphs are one Text per line
	for _, want := range []string{"pacenotch v9", "github.com/carlok/pacenotch", "Not affiliated with, or endorsed by, Anthropic", "MIT License"} {
		if !strings.Contains(all, want) {
			t.Errorf("about is missing %q in\n%s", want, all)
		}
	}
}

func TestNotifyWhenWeeklyGoesAhead(t *testing.T) {
	a := test.NewTempApp(t)
	u := NewUI(a, "test")
	test.AssertNotificationSent(t, fyne.NewNotification("pacenotch", NotifyText), func() {
		u.Show(sampleView(t))
	})

	a = test.NewTempApp(t)
	u = NewUI(a, "test")
	u.Update(func(s *Settings) { s.Notify = false })
	test.AssertNotificationSent(t, nil, func() {
		u.Show(sampleView(t))
	})
}
