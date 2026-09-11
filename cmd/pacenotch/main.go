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
	args := os.Args[1:]
	var code int
	if len(args) > 0 && args[0] == "gui" {
		code = runGUI(ctx, args[1:])
	} else {
		code = tui.Main(tui.DefaultDeps(ctx, args, version))
	}
	stop()
	os.Exit(code)
}
