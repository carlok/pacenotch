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
