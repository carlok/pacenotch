package instance

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// shortDir is a temp directory with a short path: unix socket paths are limited.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestSocketPath(t *testing.T) {
	if got := SocketPath("/c", "/t"); got != filepath.Join("/c", "pacenotch", "gui.sock") {
		t.Errorf("short: %s", got)
	}
	long := "/" + strings.Repeat("x", 120)
	got := SocketPath(long, "/t")
	if !strings.HasPrefix(got, filepath.Join("/t", "pacenotch-")) || !strings.HasSuffix(got, ".sock") || len(got) > maxSocketPath || got != SocketPath(long, "/t") {
		t.Errorf("long: %s", got)
	}
	if SocketPath(long+"y", "/t") == got {
		t.Error("different cache dirs need different sockets")
	}
}

func TestSecondInstanceShowsTheFirst(t *testing.T) {
	path := filepath.Join(shortDir(t), "p", "gui.sock")
	shows := make(chan struct{}, 4)
	first, ok, err := Acquire(path, func() { shows <- struct{}{} })
	if err != nil || !ok || first == nil {
		t.Fatalf("first: %v %v %v", first, ok, err)
	}
	if st, err := os.Stat(filepath.Dir(path)); err != nil || (st.Mode().Perm() != 0o700 && os.PathSeparator == '/') {
		t.Errorf("socket directory: %v %v", st, err)
	}

	second, ok, err := Acquire(path, func() { t.Error("the second instance must not listen") })
	if err != nil || ok || second != nil {
		t.Fatalf("second: %v %v %v", second, ok, err)
	}
	select {
	case <-shows:
	case <-time.After(5 * time.Second):
		t.Fatal("the first instance was not asked to show its window")
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, ok, err := Acquire(path, func() {})
	if err != nil || !ok {
		t.Fatalf("after the first quits, a new one takes over: %v %v", ok, err)
	}
	third.Close()
}

func TestStaleSocketFile(t *testing.T) {
	path := filepath.Join(shortDir(t), "gui.sock")
	os.WriteFile(path, []byte("left behind"), 0o600)
	l, ok, err := Acquire(path, func() {})
	if err != nil || !ok {
		t.Fatalf("stale file: %v %v", ok, err)
	}
	l.Close()
}

func TestBadRequestIsIgnored(t *testing.T) {
	path := filepath.Join(shortDir(t), "gui.sock")
	var shows atomic.Int32
	l, _, err := Acquire(path, func() { shows.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("hello\n"))
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if b, _ := io.ReadAll(conn); len(b) != 0 || shows.Load() != 0 {
		t.Errorf("unexpected answer %q, shows %d", b, shows.Load())
	}
	conn.Close()
}

func TestNotResponding(t *testing.T) {
	defer func(d time.Duration) { Timeout = d }(Timeout)
	Timeout = 200 * time.Millisecond
	path := filepath.Join(shortDir(t), "gui.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close() // never answers
		}
	}()
	if l, ok, err := Acquire(path, func() {}); !errors.Is(err, ErrNotResponding) || ok || l != nil {
		t.Errorf("got %v %v %v", l, ok, err)
	}
}

func TestAcquireErrors(t *testing.T) {
	dir := shortDir(t)
	file := filepath.Join(dir, "file")
	os.WriteFile(file, nil, 0o600)
	if _, ok, err := Acquire(filepath.Join(file, "gui.sock"), func() {}); err == nil || ok {
		t.Errorf("a file as the directory: %v %v", ok, err)
	}
	long := filepath.Join(dir, strings.Repeat("d", 60), strings.Repeat("e", 60), "gui.sock")
	if _, ok, err := Acquire(long, func() {}); err == nil || ok {
		t.Errorf("a path too long for a socket: %v %v", ok, err)
	}
}
