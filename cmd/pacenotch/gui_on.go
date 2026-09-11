//go:build gui

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlok/pacenotch/internal/gui"
)

func runGUI(ctx context.Context, args []string) int {
	return gui.Main(ctx, args, version, os.Stdout, os.Stderr)
}

// launchedAsApp reports whether the binary runs inside a macOS .app bundle, started from
// Finder with no arguments to ask for the GUI.
func launchedAsApp() bool {
	exe, err := os.Executable()
	return err == nil && strings.Contains(filepath.ToSlash(exe), ".app/Contents/MacOS/")
}
