package usage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	at := time.Unix(1789126200, 0)
	c := &Cache{Path: filepath.Join(t.TempDir(), "pacenotch", "usage.json"), Now: func() time.Time { return at }}
	if _, _, err := c.Read(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing cache: %v", err)
	}
	if err := c.Write([]byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Write([]byte(`{"a":2}`)); err != nil { // replaces an existing file
		t.Fatal(err)
	}
	b, mtime, err := c.Read()
	if err != nil || string(b) != `{"a":2}` || !mtime.Equal(at) {
		t.Fatalf("Read = %q, %v, %v", b, mtime, err)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(c.Path)
		if st.Mode().Perm() != 0o600 {
			t.Errorf("cache mode %v, want 0600", st.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(c.Path))
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

func TestCacheEmptyAndDirectory(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.json")
	os.WriteFile(empty, nil, 0o600)
	for _, p := range []string{empty, dir} {
		c := &Cache{Path: p, Now: time.Now}
		if _, _, err := c.Read(); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Read(%s) = %v, want ErrNotExist", p, err)
		}
	}
}

func TestCacheWriteErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	os.WriteFile(file, []byte("x"), 0o600)

	// the parent "directory" is a regular file
	c := &Cache{Path: filepath.Join(file, "usage.json"), Now: time.Now}
	if err := c.Write([]byte("{}")); err == nil {
		t.Error("MkdirAll under a file must fail")
	}

	// the target is an existing non-empty directory: rename fails, the temp file is removed
	target := filepath.Join(dir, "target")
	os.MkdirAll(filepath.Join(target, "child"), 0o700)
	c = &Cache{Path: target, Now: time.Now}
	if err := c.Write([]byte("{}")); err == nil {
		t.Error("rename onto a directory must fail")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".usage-") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}

	// Chtimes with a time the filesystem cannot store
	c = &Cache{Path: filepath.Join(dir, "bad-time", "usage.json"), Now: func() time.Time { return time.Date(99999, 1, 1, 0, 0, 0, 0, time.UTC) }}
	if err := c.Write([]byte("{}")); err == nil && runtime.GOOS == "windows" {
		t.Log("filesystem accepted the time; nothing to check")
	}

	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		ro := filepath.Join(dir, "ro")
		os.MkdirAll(ro, 0o500)
		c = &Cache{Path: filepath.Join(ro, "usage.json"), Now: time.Now}
		if err := c.Write([]byte("{}")); err == nil {
			t.Error("CreateTemp in a read-only directory must fail")
		}
	}
}

func TestDefaultCachePath(t *testing.T) {
	p, err := DefaultCachePath()
	if err != nil {
		t.Skipf("no cache dir on this machine: %v", err)
	}
	if filepath.Base(p) != "usage.json" || filepath.Base(filepath.Dir(p)) != "pacenotch" {
		t.Errorf("DefaultCachePath = %s", p)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("LocalAppData", "")
	if _, err := DefaultCachePath(); err == nil {
		t.Error("expected an error without HOME / XDG_CACHE_HOME / LocalAppData")
	}
}
