//go:build windows

package autostart

import (
	"errors"
	"io/fs"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// TestRunKey exercises the real registry adapter against a throwaway key.
func TestRunKey(t *testing.T) {
	k := runKey{path: `Software\pacenotch-test`}
	registry.DeleteKey(registry.CURRENT_USER, k.path)
	t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, k.path) })

	if err := k.Delete("x"); err != nil {
		t.Fatalf("delete from a missing key: %v", err)
	}
	if _, err := k.Get("x"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("get from a missing key: %v", err)
	}
	if err := k.Set("x", `"C:\p\pacenotch-gui.exe"`); err != nil {
		t.Fatal(err)
	}
	if v, err := k.Get("x"); err != nil || v != `"C:\p\pacenotch-gui.exe"` {
		t.Fatalf("get: %q %v", v, err)
	}
	if _, err := k.Get("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing value: %v", err)
	}
	if err := k.Delete("x"); err != nil {
		t.Fatal(err)
	}
	if err := k.Delete("x"); err != nil {
		t.Fatalf("deleting twice: %v", err)
	}
	if defaultRegistry().(runKey).path != runKeyPath {
		t.Error("the default registry is the Run key")
	}
}
