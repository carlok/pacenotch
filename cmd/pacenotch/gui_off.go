//go:build !gui

package main

import (
	"context"
	"fmt"
	"os"
)

func runGUI(context.Context, []string) int {
	fmt.Fprintln(os.Stderr, "pacenotch: this is the CLI-only build; `pacenotch gui` needs the GUI build")
	return 1
}

// launchedAsApp: the CLI-only build is never packaged as an app.
func launchedAsApp() bool { return false }
