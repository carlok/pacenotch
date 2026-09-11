//go:build windows

package tui

import (
	"context"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// resizeEvents polls the console size: Windows has no SIGWINCH.
func resizeEvents(ctx context.Context, size func() (int, error)) <-chan struct{} {
	return pollResize(ctx, size, 250*time.Millisecond)
}

// enableVT turns on virtual-terminal processing so the console interprets ANSI colors and
// cursor sequences. It reports false when f is not a console or the mode cannot be set.
func enableVT(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
