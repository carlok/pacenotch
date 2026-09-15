//go:build gui && darwin

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

#pragma clang diagnostic ignored "-Wdeprecated-declarations"

extern void pacenotchReopened(void); // reopen_darwin_fyne.go

// A menu bar app (LSUIElement, or the accessory policy) is not activated when it launches,
// and since macOS 14 activation is cooperative: [NSApp activate] can be refused when the
// launching app keeps focus. orderFrontRegardless still raises the window above other
// apps' windows, so it never opens hidden behind them.
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

// Opening the app while it runs (double-click in Finder, or a click on the Dock icon) sends
// kAEReopenApplication. Fyne does not handle it.
@interface PacenotchReopenHandler : NSObject
- (void)handleReopen:(NSAppleEventDescriptor *)event withReply:(NSAppleEventDescriptor *)reply;
@end

@implementation PacenotchReopenHandler
- (void)handleReopen:(NSAppleEventDescriptor *)event withReply:(NSAppleEventDescriptor *)reply {
	pacenotchReopened();
}
@end

static void pacenotchHandleReopen(void) {
	static PacenotchReopenHandler *handler;
	handler = [PacenotchReopenHandler new];
	[[NSAppleEventManager sharedAppleEventManager] setEventHandler:handler
	                                                 andSelector:@selector(handleReopen:withReply:)
	                                               forEventClass:kCoreEventClass
	                                                  andEventID:kAEReopenApplication];
}

// Regular: Dock icon, Cmd-Tab and the app menu with Quit. Accessory: menu bar only.
static void pacenotchApplyPolicy(bool dock) {
	[NSApp setActivationPolicy:(dock ? NSApplicationActivationPolicyRegular
	                                 : NSApplicationActivationPolicyAccessory)];
}

static void pacenotchStart(bool dock) {
	// run after Fyne has created and shown the first window (and after NSApplication has
	// installed its own Apple Event handlers); the second pass covers a slow first frame
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 300 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
		pacenotchApplyPolicy(dock);
		pacenotchHandleReopen();
		pacenotchBringToFront();
	});
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 1000 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
		pacenotchBringToFront();
	});
}

static void pacenotchSetDock(bool dock) {
	dispatch_async(dispatch_get_main_queue(), ^{
		pacenotchApplyPolicy(dock);
		pacenotchBringToFront(); // switching the policy can drop focus
	});
}

static void pacenotchActivate(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ pacenotchBringToFront(); });
}
*/
import "C"

// startMacApp applies the Dock mode (Regular or menu-bar-only), brings the window opened at
// launch to the front and starts listening for the app being opened again.
func startMacApp(dock bool) { C.pacenotchStart(C.bool(dock)) }

// setDockVisible switches between Dock mode and menu-bar-only while running.
func setDockVisible(on bool) { C.pacenotchSetDock(C.bool(on)) }

// bringToFront raises the app's windows: a menu bar app opens them behind the active app.
func bringToFront() { C.pacenotchActivate() }
