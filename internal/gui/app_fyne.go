//go:build gui

package gui

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/carlok/pacenotch/internal/claudecli"
	"github.com/carlok/pacenotch/internal/tui"
	"github.com/carlok/pacenotch/internal/usage"
)

// Main runs `pacenotch gui` (and pacenotch-gui.exe) and returns the exit code.
func Main(ctx context.Context, args []string, version string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "pacenotch: %v\n", err)
		return 1
	}
	opts, overrides, err := ParseArgs(args)
	if err != nil {
		return fail(err)
	}
	if opts.Help {
		io.WriteString(stdout, Help)
		return 0
	}
	now, err := tui.NowFunc(os.Getenv)
	if err != nil {
		return fail(err)
	}
	loader, err := usage.NewLoader(opts.From, os.Stdin, time.Duration(opts.TTL)*time.Second, now)
	if err != nil {
		return fail(err)
	}
	loader.Backoff = true

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a := app.NewWithID(AppID)
	a.SetIcon(fyne.NewStaticResource("pacenotch.png", EncodePNG(AppIcon(256))))
	ui := NewUI(a, version)
	ui.ApplyOverrides(overrides)
	settings := ui.Settings.Effective()
	loader.SetTTL(time.Duration(settings.TTL) * time.Second)

	setRefresh := func(bool) {}
	if loader.Source != nil {
		// stop calling the endpoint while an expired token is unchanged, and (setting on)
		// let Claude Code refresh it
		recovering := usage.NewRecoveringSource(loader.Source, nil, now)
		loader.Source = recovering
		refresher := claudecli.NewRefresher()
		setRefresh = func(on bool) {
			if on {
				recovering.SetRefresh(refresher.Refresh)
			} else {
				recovering.SetRefresh(nil)
			}
		}
	}
	setRefresh(settings.RefreshToken)
	ctrl := &Controller{Load: loader.Load, Now: now, Band: settings.Band, Publish: ui.Publish}
	ui.OnSettings = func(before, after Settings) {
		if after.TTL != before.TTL {
			loader.SetTTL(time.Duration(after.TTL) * time.Second)
		}
		if after.RefreshToken != before.RefreshToken {
			setRefresh(after.RefreshToken)
		}
		if after.Band != before.Band {
			go ctrl.SetBand(after.Band)
		}
	}
	ui.OnRefresh = func() { go ctrl.Refresh(ctx, true) }
	setReopenHandler(func() { fyne.Do(ui.ShowWindow) }) // opened again while running
	a.Lifecycle().SetOnStarted(func() {
		startMacApp(ui.ShowInDock())
		go ctrl.Run(ctx, time.Minute, time.After) // redraw every 60 s, fetch per TTL
	})
	go func() {
		<-ctx.Done() // Ctrl-C in the launching terminal
		fyne.Do(a.Quit)
	}()
	ui.Start()
	a.Run()
	return 0
}
