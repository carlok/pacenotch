//go:build gui

package gui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

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
		"resets in 2d 13h", "at this rate: limit in 1d 17h, 20h 4m before reset", "Refresh now", "About"} {
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
		"5h  33% / pace 50%  ▼ room to spare  · resets 2h 30m", "Open window", "Refresh now", "Appearance", "About pacenotch", "Quit"} {
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
	if a.Preferences().String(prefTheme) != ThemeSystem {
		t.Error("the choice must be saved")
	}
	for _, it := range u.Menu().Items {
		if it.Label == "Appearance" {
			if it.ChildMenu.Items[0].Checked || !it.ChildMenu.Items[1].Checked {
				t.Error("System must be checked")
			}
			it.ChildMenu.Items[0].Action()
		}
	}
	if u.ThemeChoice() != ThemeDark {
		t.Error("the Dark menu item must switch back")
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
	if !a.Preferences().Bool(prefCompact) || u.Window.Canvas().Size().Width > windowSize(false).Width {
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
}
