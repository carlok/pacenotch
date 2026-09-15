//go:build !windows

package autostart

// defaultRegistry: only Windows has the Run key.
func defaultRegistry() Registry { return nil }
