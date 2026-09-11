//go:build gui && darwin

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

#pragma clang diagnostic ignored "-Wdeprecated-declarations"

// A menu bar app (LSUIElement, or the accessory policy set below) is not activated when it
// launches, and since macOS 14 activation is cooperative: [NSApp activate] can be refused
// when the launching app keeps focus. orderFrontRegardless still raises the window above
// other apps' windows, so it never opens hidden behind them.
static void pacenotchBringToFront(void) {
	if (@available(macOS 14.0, *)) {
		[NSApp activate];
	} else {
		[NSApp activateIgnoringOtherApps:YES];
	}
	for (NSWindow *w in [NSApp windows]) {
		if ([w isVisible]) {
			[w makeKeyAndOrderFront:nil];
			[w orderFrontRegardless];
		}
	}
}

static void pacenotchMenuBarApp(void) {
	// run after Fyne has created and shown the first window; the second pass covers a
	// slow first frame
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 300 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		pacenotchBringToFront();
	});
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 1000 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
		pacenotchBringToFront();
	});
}

static void pacenotchActivate(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ pacenotchBringToFront(); });
}
*/
import "C"

// hideDockIcon makes the process a menu bar app with no Dock icon, like LSUIElement does
// for the .app bundle, and brings the window opened at launch to the front.
func hideDockIcon() { C.pacenotchMenuBarApp() }

// bringToFront raises the app's windows: a menu bar app opens them behind the active app.
func bringToFront() { C.pacenotchActivate() }
