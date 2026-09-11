//go:build gui && darwin

package gui

import "C"

import "sync/atomic"

var reopenHandler atomic.Pointer[func()]

// setReopenHandler sets what runs when the app is opened again while already running
// (double-click in Finder, Spotlight, Launchpad): macOS sends a reopen event, not a launch.
func setReopenHandler(f func()) { reopenHandler.Store(&f) }

//export pacenotchReopened
func pacenotchReopened() {
	if f := reopenHandler.Load(); f != nil {
		(*f)()
	}
}
