//go:build windows

package tui

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnableVT(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "not-a-console"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if enableVT(f) {
		t.Error("a regular file is not a console")
	}
	t.Logf("stdout VT processing: %v", enableVT(os.Stdout)) // a console only when run interactively
}

func TestResizeEventsPolling(t *testing.T) {
	var width atomic.Int64
	width.Store(80)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := resizeEvents(ctx, func() (int, error) { return int(width.Load()), nil })
	time.Sleep(300 * time.Millisecond)
	width.Store(120)
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no resize event")
	}
}
