//go:build gui && !darwin

package gui

// startMacApp: only macOS has a Dock and menu-bar-only apps.
func startMacApp(bool) {}

// setDockVisible: only macOS has a Dock mode.
func setDockVisible(bool) {}

// bringToFront: Window.RequestFocus is enough outside macOS.
func bringToFront() {}

// setReopenHandler: only macOS reopens a running app instead of launching a new one.
func setReopenHandler(func()) {}
