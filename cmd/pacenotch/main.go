// Command pacenotch shows Claude plan usage limits with a vertical pace notch.
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
	code := tui.Main(tui.DefaultDeps(ctx, os.Args[1:], version))
	stop()
	os.Exit(code)
}
