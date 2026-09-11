//go:build gui

package gui

import (
	"image"
	"image/color"
	"net/url"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/carlok/pacenotch/internal/pace"
	"github.com/carlok/pacenotch/internal/tui"
)

// Theme preference values.
const (
	prefTheme   = "theme"
	ThemeDark   = "dark"
	ThemeSystem = "system"
)

// UI is the Fyne glue: it only turns Views into widgets, the tray menu and the tray icon.
type UI struct {
	App       fyne.App
	Window    fyne.Window
	Version   string
	OnRefresh func() // "Refresh now"

	view     View
	notifier AheadNotifier
	about    fyne.Window
}

// NewUI creates the main window (hidden) and applies the saved theme choice.
func NewUI(a fyne.App, version string) *UI {
	u := &UI{App: a, Version: version, view: Loading}
	u.applyTheme()
	u.Window = a.NewWindow("pacenotch")
	u.Window.SetCloseIntercept(u.Window.Hide) // closing the window leaves the tray running
	u.Window.Resize(fyne.NewSize(600, 460))
	a.Settings().AddListener(func(fyne.Settings) { u.render() }) // follow OS light/dark changes
	return u
}

// Start builds the tray and shows the window.
func (u *UI) Start() {
	u.render()
	u.Window.Show()
}

// Publish shows a new view; safe from any goroutine.
func (u *UI) Publish(v View) { fyne.Do(func() { u.Show(v) }) }

// Show shows a new view; call on the Fyne goroutine.
func (u *UI) Show(v View) {
	u.view = v
	if u.notifier.Update(v) {
		u.App.SendNotification(fyne.NewNotification("pacenotch", NotifyText))
	}
	u.render()
}

func (u *UI) render() {
	u.Window.SetContent(u.content())
	if desk, ok := u.App.(desktop.App); ok {
		desk.SetSystemTrayMenu(u.Menu())
		desk.SetSystemTrayIcon(u.TrayResource())
	}
}

// ThemeChoice is "dark" (the default) or "system".
func (u *UI) ThemeChoice() string {
	return u.App.Preferences().StringWithFallback(prefTheme, ThemeDark)
}

// SetThemeChoice saves and applies "dark" or "system".
func (u *UI) SetThemeChoice(choice string) {
	if choice == u.ThemeChoice() {
		return
	}
	u.App.Preferences().SetString(prefTheme, choice)
	u.applyTheme()
	u.render()
}

func (u *UI) applyTheme() {
	if u.ThemeChoice() == ThemeDark {
		u.App.Settings().SetTheme(forcedVariant{theme.DefaultTheme(), theme.VariantDark})
	} else {
		u.App.Settings().SetTheme(theme.DefaultTheme())
	}
}

// Dark reports whether the window renders dark. It looks at the background color the
// theme actually draws, so the bar palette always matches it.
func (u *UI) Dark() bool {
	s := u.App.Settings()
	bg := color.NRGBAModel.Convert(s.Theme().Color(theme.ColorNameBackground, s.ThemeVariant())).(color.NRGBA)
	return luminance(bg) < 0.5
}

type forcedVariant struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

func (f forcedVariant) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return f.Theme.Color(n, f.variant)
}

// TrayResource renders the tray icon: 2:1 in the macOS menu bar, square elsewhere. It uses
// the OS variant, because the menu bar and taskbar follow the OS, not the app.
func (u *UI) TrayResource() fyne.Resource {
	wide := runtime.GOOS == "darwin"
	dark := u.App.Settings().ThemeVariant() == theme.VariantDark
	img := TrayIcon(u.view.PaceRows(), u.view.IconState(), 32, wide, Bars(dark))
	return fyne.NewStaticResource("pacenotch-tray.png", EncodePNG(img))
}

// Menu is the tray menu: one line per window, then the actions.
func (u *UI) Menu() *fyne.Menu {
	var items []*fyne.MenuItem
	if u.view.Error != "" {
		items = append(items, fyne.NewMenuItem("⚠ "+u.view.Error, u.ShowWindow))
	} else if u.view.Stale {
		items = append(items, fyne.NewMenuItem("⚠ "+u.view.Header, u.ShowWindow))
	}
	for _, r := range u.view.Rows {
		items = append(items, fyne.NewMenuItem(r.Menu, u.ShowWindow))
	}
	dark := fyne.NewMenuItem("Dark", func() { u.SetThemeChoice(ThemeDark) })
	system := fyne.NewMenuItem("System", func() { u.SetThemeChoice(ThemeSystem) })
	dark.Checked, system.Checked = u.ThemeChoice() == ThemeDark, u.ThemeChoice() == ThemeSystem
	appearance := fyne.NewMenuItem("Appearance", nil)
	appearance.ChildMenu = fyne.NewMenu("", dark, system)
	quit := fyne.NewMenuItem("Quit", u.App.Quit)
	quit.IsQuit = true
	items = append(items, fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Open window", u.ShowWindow),
		fyne.NewMenuItem("Refresh now", u.refresh),
		appearance,
		fyne.NewMenuItem("About pacenotch", u.ShowAbout),
		fyne.NewMenuItemSeparator(), quit)
	return fyne.NewMenu("pacenotch", items...)
}

// ShowWindow brings the main window up.
func (u *UI) ShowWindow() {
	u.Window.Show()
	u.Window.RequestFocus()
}

func (u *UI) refresh() {
	if u.OnRefresh != nil {
		u.OnRefresh()
	}
}

func (u *UI) content() fyne.CanvasObject {
	th := Bars(u.Dark())
	v := u.view
	header := container.NewBorder(nil, nil, styled("pacenotch", th.Text, true), styled(v.Header, tone(v.HeaderTone, th), false))

	body := container.NewVBox()
	if v.Error != "" {
		body.Add(styled(v.Error, tone(ToneWarn, th), false))
	}
	if v.NoWindows {
		body.Add(styled(tui.NoWindows, th.Dim, false))
	}
	for _, r := range v.Rows {
		title := container.NewHBox(styled(r.Title, th.Text, true), styled(r.Status, th.Fill[r.Row.Class], false))
		body.Add(container.NewBorder(nil, nil, title, styled(r.Resets, th.Dim, false)))
		body.Add(Bar(r.Row, th))
		body.Add(styled(r.Budget, th.Dim, false))
		if r.Projection != "" {
			body.Add(styled(r.Projection, tone(r.ProjTone, th), false))
		}
		body.Add(layout.NewSpacer())
	}
	if v.Extra != "" {
		body.Add(styled(v.Extra, th.Dim, false))
	}
	if v.Warn != "" {
		body.Add(styled(v.Warn, tone(ToneWarn, th), false))
	}

	choice := widget.NewSelect([]string{"Dark", "System"}, nil)
	if u.ThemeChoice() == ThemeDark {
		choice.SetSelected("Dark")
	} else {
		choice.SetSelected("System")
	}
	choice.OnChanged = func(s string) {
		if s == "Dark" {
			u.SetThemeChoice(ThemeDark)
		} else {
			u.SetThemeChoice(ThemeSystem)
		}
	}
	footer := container.NewHBox(
		widget.NewButtonWithIcon("Refresh now", theme.ViewRefreshIcon(), u.refresh),
		layout.NewSpacer(), widget.NewLabel("Appearance"), choice,
		widget.NewButtonWithIcon("About", theme.InfoIcon(), u.ShowAbout))
	return container.NewBorder(container.NewPadded(header), container.NewPadded(footer), nil, nil,
		container.NewVScroll(container.NewPadded(body)))
}

// Bar is a horizontal bar with its vertical pace notch that stretches with its container.
func Bar(row pace.Row, th BarTheme) *canvas.Raster {
	r := canvas.NewRaster(func(w, h int) image.Image {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		DrawBar(img, img.Bounds(), row, th)
		return img
	})
	r.SetMinSize(fyne.NewSize(160, 22))
	return r
}

func styled(text string, c color.Color, bold bool) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextStyle.Bold = bold
	return t
}

// paragraph is word-wrapped text as tight lines (wrapping labels leave gaps in a VBox).
func paragraph(text string, c color.Color) fyne.CanvasObject {
	box := container.New(layout.NewCustomPaddedVBoxLayout(2))
	for _, line := range Wrap(text, 56) {
		box.Add(styled(line, c, false))
	}
	return box
}

func tone(t Tone, th BarTheme) color.Color {
	switch t {
	case ToneDim:
		return th.Dim
	case ToneWarn:
		return th.Fill[pace.On]
	case ToneAlert:
		return th.Fill[pace.Ahead]
	}
	return th.Text
}

// ShowAbout opens the About / credits window.
func (u *UI) ShowAbout() {
	if u.about == nil {
		th := Bars(u.Dark())
		lines := AboutLines(u.Version)
		repo, _ := url.Parse(RepoURL)
		issues, _ := url.Parse(IssuesURL)
		sample := pace.Row{Class: pace.Ahead, UsedBP: 7200, ExpBP: 6339}
		u.about = u.App.NewWindow("About pacenotch")
		u.about.SetCloseIntercept(u.about.Hide)
		u.about.SetContent(container.NewPadded(container.NewVBox(
			styled(lines[0], th.Text, true),
			styled(lines[1], th.Dim, false),
			Bar(sample, th),
			paragraph(lines[2], th.Text),
			paragraph(lines[3], tone(ToneWarn, th)),
			container.NewHBox(widget.NewHyperlink("github.com/carlok/pacenotch", repo), widget.NewHyperlink("Report an issue", issues)),
			paragraph(lines[4], th.Dim),
			styled(lines[5], th.Dim, false),
		)))
		u.about.Resize(fyne.NewSize(480, 380))
	}
	u.about.Show()
	u.about.RequestFocus()
}
