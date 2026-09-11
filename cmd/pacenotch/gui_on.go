//go:build gui

package main

import (
	"context"
	"os"

	"github.com/carlok/pacenotch/internal/gui"
)

func runGUI(ctx context.Context, args []string) int {
	return gui.Main(ctx, args, version, os.Stdout, os.Stderr)
}
