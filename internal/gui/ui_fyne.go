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
	"github.com/carlok/pacenotch/internal/update"
)

// QuitShortcut is Cmd-Q on macOS and Ctrl-Q elsewhere.
var QuitShortcut = &desktop.CustomShortcut{KeyName: fyne.KeyQ, Modifier: fyne.KeyModifierShortcutDefault}

// windowSize is the window size for the full and the compact view.
func windowSize(compact bool) fyne.Size {
	if compact {
		return fyne.NewSize(460, 230)
	}
	return fyne.NewSize(600, 460)
}

// UI is the Fyne glue: it only turns Views and Settings into widgets, the tray menu and the
// tray icon.
type UI struct {
	App       fyne.App
	Window    fyne.Window
	Version   string
	Settings  *SettingsModel
	Autostart Autostart // start at login; nil hides the setting
	OnRefresh func()    // "Refresh now"
	Quit      func()    // the window's Quit button and Cmd/Ctrl-Q; App.Quit by default

	// OnSettings is called after the settings changed, with the effective values before and
	// after, so the app can apply the band, the TTL, token refresh and so on.
	OnSettings func(before, after Settings)

	view        View
	notifier    AheadNotifier
	about       fyne.Window
	settingsWin fyne.Window
	loginErr    error          // the last failed change of the login item
	update      update.Release // a newer release, when one is known
}

// SetUpdate shows (or, with an empty release, hides) the "update available" notice.
func (u *UI) SetUpdate(rel update.Release) {
	u.update = rel
	u.render()
}

func (u *UI) openUpdate() {
	if link, err := url.Parse(u.update.URL); err == nil {
		u.App.OpenURL(link)
	}
}

// NewUI creates the main window (hidden) and applies the saved settings.
func NewUI(a fyne.App, version string) *UI {
	u := &UI{App: a, Version: version, view: Loading, Quit: a.Quit, Settings: NewSettingsModel(a.Preferences(), Overrides{})}
	u.applyTheme()
	u.Window = a.NewWindow("pacenotch")
	u.Window.SetCloseIntercept(u.Window.Hide) // closing the window leaves the tray running
	u.Window.Resize(windowSize(u.Compact()))
	// Cmd-Q on macOS, Ctrl-Q elsewhere: quits even when the menu bar icon is hidden
	u.Window.Canvas().AddShortcut(QuitShortcut, func(fyne.Shortcut) { u.Quit() })
	if runtime.GOOS == "darwin" {
		// "Settings…" moves into the app menu (pacenotch → Settings…, Cmd-,)
		u.Window.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("View",
			fyne.NewMenuItem("Refresh now", u.refresh),
			fyne.NewMenuItem("Settings…", u.ShowSettings))))
	}
	a.Settings().AddListener(func(fyne.Settings) { u.render() }) // follow OS light/dark changes
	return u
}

// ApplyOverrides sets this session's command-line overrides.
func (u *UI) ApplyOverrides(o Overrides) {
	u.Settings = NewSettingsModel(u.App.Preferences(), o)
	u.applyTheme()
	u.Window.Resize(windowSize(u.Compact()))
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
	if u.notifier.Update(v) && u.Settings.Effective().Notify {
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

// Update changes the settings, applies what the UI owns (theme, window size, Dock mode),
// tells the app through OnSettings and redraws.
func (u *UI) Update(change func(*Settings)) error {
	before, after, err := u.Settings.Update(change)
	if err != nil || before == after {
		return err
	}
	if after.Theme != before.Theme {
		u.applyTheme()
	}
	if after.Compact != before.Compact {
		u.Window.Resize(windowSize(after.Compact))
	}
	if after.ShowInDock != before.ShowInDock {
		setDockVisible(after.ShowInDock)
	}
	if u.OnSettings != nil {
		u.OnSettings(before, after)
	}
	u.render()
	if u.settingsWin != nil {
		u.settingsWin.SetContent(u.settingsContent())
	}
	return nil
}

// ThemeChoice is "dark" (the default) or "system".
func (u *UI) ThemeChoice() string { return u.Settings.Effective().Theme }

// SetThemeChoice saves and applies "dark" or "system".
func (u *UI) SetThemeChoice(choice string) { u.Update(func(s *Settings) { s.Theme = choice }) }

// Compact reports whether the window shows one line per window.
func (u *UI) Compact() bool { return u.Settings.Effective().Compact }

// SetCompact saves the compact choice, resizes the window to fit and redraws.
func (u *UI) SetCompact(on bool) { u.Update(func(s *Settings) { s.Compact = on }) }

// ShowInDock reports whether pacenotch shows in the Dock and Cmd-Tab (macOS).
func (u *UI) ShowInDock() bool { return u.Settings.Effective().ShowInDock }

// SetShowInDock saves the Dock mode and applies it right away.
func (u *UI) SetShowInDock(on bool) { u.Update(func(s *Settings) { s.ShowInDock = on }) }

// RefreshToken reports whether an expired token is refreshed through Claude Code.
func (u *UI) RefreshToken() bool { return u.Settings.Effective().RefreshToken }

// SetRefreshToken saves the setting; OnSettings tells the loader.
func (u *UI) SetRefreshToken(on bool) { u.Update(func(s *Settings) { s.RefreshToken = on }) }

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
	if text := UpdateText(u.update); text != "" {
		items = append(items, fyne.NewMenuItemSeparator(), fyne.NewMenuItem(text, u.openUpdate))
	}
	compact := fyne.NewMenuItem("Compact window", func() { u.SetCompact(!u.Compact()) })
	compact.Checked = u.Compact()
	quit := fyne.NewMenuItem("Quit", u.App.Quit)
	quit.IsQuit = true
	items = append(items, fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Open window", u.ShowWindow),
		fyne.NewMenuItem("Refresh now", u.refresh),
		compact,
		fyne.NewMenuItem("Settings…", u.ShowSettings),
		fyne.NewMenuItem("About pacenotch", u.ShowAbout),
		fyne.NewMenuItemSeparator(), quit)
	return fyne.NewMenu("pacenotch", items...)
}

// ShowWindow brings the main window up.
func (u *UI) ShowWindow() {
	u.Window.Show()
	u.Window.RequestFocus()
	bringToFront()
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
		if u.Compact() {
			// fixed-width columns on both sides, so every bar has the same length
			label := container.NewGridWrap(fyne.NewSize(44, 22), styled(r.Short, th.Text, true))
			tail := styled(r.Compact, th.Fill[r.Row.Class], false)
			tail.Alignment = fyne.TextAlignTrailing
			body.Add(container.NewBorder(nil, nil, label, container.NewGridWrap(fyne.NewSize(124, 22), tail), Bar(r.Row, th)))
			continue
		}
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

	compact := widget.NewCheck("Compact", nil)
	compact.SetChecked(u.Compact())
	compact.OnChanged = u.SetCompact
	quit := func() { u.Quit() }
	footer := container.NewHBox(
		widget.NewButtonWithIcon("Refresh now", theme.ViewRefreshIcon(), u.refresh),
		layout.NewSpacer(), compact,
		widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), u.ShowSettings),
		widget.NewButtonWithIcon("About", theme.InfoIcon(), u.ShowAbout),
		widget.NewButtonWithIcon("Quit", theme.CancelIcon(), quit))
	if u.Compact() { // icon-only buttons, so the footer fits the narrow window
		footer = container.NewHBox(
			widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), u.refresh),
			layout.NewSpacer(), compact,
			widget.NewButtonWithIcon("", theme.SettingsIcon(), u.ShowSettings),
			widget.NewButtonWithIcon("", theme.InfoIcon(), u.ShowAbout),
			widget.NewButtonWithIcon("", theme.CancelIcon(), quit))
	}
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
		u.about = u.App.NewWindow("About pacenotch")
		u.about.SetCloseIntercept(u.about.Hide)
		u.about.Resize(fyne.NewSize(480, 400))
	}
	th := Bars(u.Dark())
	lines := AboutLines(u.Version)
	repo, _ := url.Parse(RepoURL)
	issues, _ := url.Parse(IssuesURL)
	sample := pace.Row{Class: pace.Ahead, UsedBP: 7200, ExpBP: 6339}
	box := container.NewVBox(
		styled(lines[0], th.Text, true),
		styled(lines[1], th.Dim, false),
		Bar(sample, th),
		paragraph(lines[2], th.Text),
		paragraph(lines[3], tone(ToneWarn, th)),
		container.NewHBox(widget.NewHyperlink("github.com/carlok/pacenotch", repo), widget.NewHyperlink("Report an issue", issues)),
		paragraph(lines[4], th.Dim),
		styled(lines[5], th.Dim, false),
	)
	if text := UpdateText(u.update); text != "" {
		if link, err := url.Parse(u.update.URL); err == nil {
			box.Add(widget.NewHyperlink(text, link))
		}
	}
	u.about.SetContent(container.NewPadded(box))
	u.about.Show()
	u.about.RequestFocus()
	bringToFront()
}
