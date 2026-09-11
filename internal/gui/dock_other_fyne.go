//go:build gui && !darwin

package gui

// hideDockIcon: only macOS has a Dock icon to hide.
func hideDockIcon() {}

// bringToFront: Window.RequestFocus is enough outside macOS.
func bringToFront() {}
