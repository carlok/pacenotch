package usage

import (
	"os"
	"path/filepath"
	"time"
)

// Cache stores the last successful response. Its modification time is the fetch time.
type Cache struct {
	Path string
	Now  func() time.Time
}

// DefaultCachePath is os.UserCacheDir()/pacenotch/usage.json.
func DefaultCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pacenotch", "usage.json"), nil
}

// Read returns the cached body and when it was written. A missing or empty file is
// reported as os.ErrNotExist.
func (c *Cache) Read() ([]byte, time.Time, error) {
	st, err := os.Stat(c.Path)
	if err != nil {
		return nil, time.Time{}, os.ErrNotExist
	}
	b, err := os.ReadFile(c.Path)
	if err != nil || len(b) == 0 {
		return nil, time.Time{}, os.ErrNotExist
	}
	return b, st.ModTime(), nil
}

// Write replaces the cache atomically, readable only by the user, and stamps it with Now.
func (c *Cache) Write(data []byte) error {
	dir := filepath.Dir(c.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".usage-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, 0o600)
	}
	if werr == nil {
		now := c.Now()
		werr = os.Chtimes(name, now, now)
	}
	if werr == nil {
		werr = os.Rename(name, c.Path)
	}
	if werr != nil {
		os.Remove(name)
	}
	return werr
}
