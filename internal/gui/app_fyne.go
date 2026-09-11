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

	"github.com/carlok/pacenotch/internal/tui"
	"github.com/carlok/pacenotch/internal/usage"
)

// Main runs `pacenotch gui` (and pacenotch-gui.exe) and returns the exit code.
func Main(ctx context.Context, args []string, version string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "pacenotch: %v\n", err)
		return 1
	}
	opts, err := ParseArgs(args)
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
	ctrl := &Controller{Load: loader.Load, Now: now, Band: opts.Band, Publish: ui.Publish}
	ui.OnRefresh = func() { go ctrl.Refresh(ctx, true) }
	setReopenHandler(func() { fyne.Do(ui.ShowWindow) }) // opened again while running
	a.Lifecycle().SetOnStarted(func() {
		hideDockIcon()
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
