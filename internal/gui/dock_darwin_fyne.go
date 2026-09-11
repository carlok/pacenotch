//go:build gui && darwin

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void pacenotchHideDock(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	});
}
*/
import "C"

// hideDockIcon makes the process a menu bar app with no Dock icon, like LSUIElement does
// for the .app bundle, so `pacenotch gui` behaves the same when run from a terminal.
func hideDockIcon() { C.pacenotchHideDock() }
