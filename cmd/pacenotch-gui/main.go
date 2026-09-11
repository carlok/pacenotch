//go:build gui

// Command pacenotch-gui is the Windows GUI executable (built with -H=windowsgui, so it
// opens no console). It is the same as `pacenotch gui`.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/carlok/pacenotch/internal/gui"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := gui.Main(ctx, os.Args[1:], version, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
