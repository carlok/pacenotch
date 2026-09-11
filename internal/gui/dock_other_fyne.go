//go:build gui && !darwin

package gui

// hideDockIcon: only macOS has a Dock icon to hide.
func hideDockIcon() {}

// bringToFront: Window.RequestFocus is enough outside macOS.
func bringToFront() {}

// setReopenHandler: only macOS reopens a running app instead of launching a new one.
func setReopenHandler(func()) {}
