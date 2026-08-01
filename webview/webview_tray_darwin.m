// Compiled only on macOS. cgo hands every .m file in the package to the C
// compiler regardless of Go build tags, so guard the whole unit on __APPLE__
// to keep Linux/Windows builds (which also build package webview) clean.
#ifdef __APPLE__

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

#include "webview_tray_darwin.h"

// MassMinimizeTarget is the action target wired onto the minimize button. It
// forwards the click to Go instead of letting AppKit iconify the window, so the
// app can fold to the tray. We keep one target per window, retained by being set
// as the button's target (AppKit keeps a weak ref, so we also stash it via
// associated object to avoid premature release).
@interface MassMinimizeTarget : NSObject
@property (assign) NSWindow *window;
@end

@implementation MassMinimizeTarget
- (void)minimize:(id)sender {
	goOnMinimize((void *)self.window);
}
@end

static const char *kMassTargetKey = "mass_minimize_target";

void installMinimize(void *win) {
	NSWindow *window = (NSWindow *)win;
	if (window == nil) {
		return;
	}
	NSButton *btn = [window standardWindowButton:NSWindowMiniaturizeButton];
	if (btn == nil) {
		return;
	}
	MassMinimizeTarget *target = [[MassMinimizeTarget alloc] init];
	target.window = window;
	// Retain the target for the window's lifetime.
	objc_setAssociatedObject(window, kMassTargetKey, target,
	                         OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[btn setTarget:target];
	[btn setAction:@selector(minimize:)];
}

void hideWindow(void *win) {
	NSWindow *window = (NSWindow *)win;
	if (window != nil) {
		[window orderOut:nil];
	}
}

void showWindow(void *win) {
	NSWindow *window = (NSWindow *)win;
	if (window != nil) {
		[NSApp activateIgnoringOtherApps:YES];
		[window makeKeyAndOrderFront:nil];
	}
}

#endif // __APPLE__
