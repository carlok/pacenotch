// Command pacenotch shows Claude plan usage limits with a vertical pace notch.
// `pacenotch gui` opens the tray icon and window in GUI builds (-tags gui).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/carlok/pacenotch/internal/tui"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	var code int
	if guiArgs, ok := wantsGUI(os.Args[1:]); ok {
		code = runGUI(ctx, guiArgs)
	} else {
		code = tui.Main(tui.DefaultDeps(ctx, os.Args[1:], version))
	}
	stop()
	os.Exit(code)
}

// wantsGUI: `pacenotch gui [flags]`, or a macOS .app bundle started with no arguments.
func wantsGUI(args []string) ([]string, bool) {
	if len(args) > 0 && args[0] == "gui" {
		return args[1:], true
	}
	return nil, len(args) == 0 && launchedAsApp()
}
