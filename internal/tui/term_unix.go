//go:build !windows

package tui

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// resizeEvents turns SIGWINCH into redraw events.
func resizeEvents(ctx context.Context, _ func() (int, error)) <-chan struct{} {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	out := make(chan struct{}, 1)
	go func() {
		defer signal.Stop(sig)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sig:
				notify(out)
			}
		}
	}()
	return out
}

// enableVT: Unix terminals interpret escape sequences already.
func enableVT(*os.File) bool { return true }
