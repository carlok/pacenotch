//go:build !windows

package tui

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestResizeEventsSIGWINCH(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := resizeEvents(ctx, nil)
	deadline := time.After(5 * time.Second)
	for {
		syscall.Kill(os.Getpid(), syscall.SIGWINCH)
		select {
		case <-ch:
			if !enableVT(os.Stdout) {
				t.Error("Unix terminals need no VT setup")
			}
			return
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatal("no event for SIGWINCH")
		}
	}
}
