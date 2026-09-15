// Package instance keeps one GUI running. The first instance listens on a local unix
// socket; a second launch connects, asks it to show its window, and exits.
package instance

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// maxSocketPath stays under the unix socket path limit (104 bytes on macOS, 108 on Linux).
const maxSocketPath = 100

// Timeout bounds every exchange between two instances.
var Timeout = time.Second

// ErrNotResponding means something listens on the socket but does not answer.
var ErrNotResponding = errors.New("another pacenotch is running but does not respond")

// SocketPath is <cacheDir>/pacenotch/gui.sock, or a short path in tempDir when that would be
// too long for a unix socket.
func SocketPath(cacheDir, tempDir string) string {
	p := filepath.Join(cacheDir, "pacenotch", "gui.sock")
	if len(p) <= maxSocketPath {
		return p
	}
	sum := sha256.Sum256([]byte(cacheDir))
	return filepath.Join(tempDir, "pacenotch-"+hex.EncodeToString(sum[:4])+".sock")
}

// Lock is held by the running instance.
type Lock struct {
	ln   net.Listener
	path string
	wg   sync.WaitGroup
}

// Acquire makes this process the running instance, or asks the running one to show its
// window. first is false when another instance answered: the caller should exit. onShow runs
// on the listener's goroutine.
func Acquire(path string, onShow func()) (lock *Lock, first bool, err error) {
	if answered, err := ask(path); answered || err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	os.Remove(path) // a socket file left behind by a crash
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, false, err
	}
	l := &Lock{ln: ln, path: path}
	l.wg.Add(1)
	go l.serve(onShow)
	return l, true, nil
}

// ask returns true when a running instance answered "ok" to "show".
func ask(path string) (bool, error) {
	conn, err := net.DialTimeout("unix", path, Timeout)
	if err != nil {
		return false, nil // nobody is listening
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(Timeout))
	_, werr := conn.Write([]byte("show\n"))
	line, rerr := bufio.NewReader(conn).ReadString('\n')
	if werr != nil || rerr != nil || strings.TrimSpace(line) != "ok" {
		return false, ErrNotResponding
	}
	return true, nil
}

func (l *Lock) serve(onShow func()) {
	defer l.wg.Done()
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			return // closed
		}
		conn.SetDeadline(time.Now().Add(Timeout))
		if line, _ := bufio.NewReader(conn).ReadString('\n'); strings.TrimSpace(line) == "show" {
			onShow()
			conn.Write([]byte("ok\n"))
		}
		conn.Close()
	}
}

// Close stops listening and removes the socket.
func (l *Lock) Close() error {
	err := l.ln.Close()
	l.wg.Wait()
	os.Remove(l.path)
	return err
}
