// Compiled only on macOS. cgo hands every .m file in the package to the C
// compiler regardless of Go build tags, so guard the whole unit on __APPLE__
// to keep Linux/Windows builds (which also build package webview) clean.
#ifdef __APPLE__

#import <Cocoa/Cocoa.h>

#include "webview_menu_darwin.h"

// MassQuitTarget backs the Quit item. NSApplication's terminate: would end the
// process from under the Go runtime: Run() would never return and the caller's
// Destroy() and shutdown would not run. Closing the window instead takes the
// exact path the window's close button takes (windowWillClose: -> the engine
// stops the run loop -> Run() returns -> the caller destroys and exits), so
// Cmd+Q and clicking X behave identically.
@interface MassQuitTarget : NSObject
@property (nonatomic, retain) NSWindow *window;
@end

@implementation MassQuitTarget
- (void)quit:(id)sender {
	[self.window performClose:sender];
}
@end

// NSMenuItem does not retain its target, so the Quit target outlives the menu
// here instead.
static MassQuitTarget *gQuitTarget = nil;

// addItem appends one item. A nil target sends the action down the responder
// chain (WKWebView -> window -> NSApp), which is what lets the Edit items act
// on the page's selection. A zero mods keeps the default Command modifier.
static void addItem(NSMenu *menu, NSString *title, SEL action, NSString *key,
                    NSEventModifierFlags mods, id target) {
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title
	                                              action:action
	                                       keyEquivalent:key];
	if (mods != 0) {
		[item setKeyEquivalentModifierMask:mods];
	}
	[item setTarget:target];
	[menu addItem:item];
	[item release];
}

// addSubmenu appends a top-level menu and returns it; the returned menu is
// owned by the item that holds it.
static NSMenu *addSubmenu(NSMenu *mainMenu, NSString *title) {
	NSMenuItem *holder = [[NSMenuItem alloc] initWithTitle:title
	                                                action:NULL
	                                         keyEquivalent:@""];
	NSMenu *submenu = [[NSMenu alloc] initWithTitle:title];
	[holder setSubmenu:submenu];
	[submenu release];
	[mainMenu addItem:holder];
	[holder release];
	return submenu;
}

void installMainMenu(void *win) {
	// Called before the run loop starts, so there is no pool of its own yet.
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		// Keyed on our own install, not on [app mainMenu]: when launched from
		// Finder, LaunchServices has already put a placeholder main menu (one
		// app-named item with an empty submenu) in place, so a non-nil mainMenu
		// would skip the install and leave every Command shortcut dead.
		if (gQuitTarget != nil) {
			return;
		}

		NSString *appName =
		    [[[NSBundle mainBundle] infoDictionary] objectForKey:@"CFBundleName"];
		if ([appName length] == 0) {
			appName = [[NSProcessInfo processInfo] processName];
		}

		NSMenu *mainMenu = [[NSMenu alloc] initWithTitle:@""];

		NSMenu *appMenu = addSubmenu(mainMenu, appName);
		addItem(appMenu, [@"Hide " stringByAppendingString:appName],
		        @selector(hide:), @"h", 0, nil);
		addItem(appMenu, @"Hide Others", @selector(hideOtherApplications:), @"h",
		        NSEventModifierFlagOption | NSEventModifierFlagCommand, nil);
		[appMenu addItem:[NSMenuItem separatorItem]];
		gQuitTarget = [[MassQuitTarget alloc] init];
		gQuitTarget.window = (NSWindow *)win;
		addItem(appMenu, [@"Quit " stringByAppendingString:appName],
		        @selector(quit:), @"q", 0, gQuitTarget);

		// No Find item: a menu key equivalent is matched before the key event
		// reaches the web content, and Cmd+F belongs to the page's own find bar.
		// No Close item either — Cmd+W closes the app's in-page tabs.
		NSMenu *editMenu = addSubmenu(mainMenu, @"Edit");
		addItem(editMenu, @"Undo", @selector(undo:), @"z", 0, nil);
		addItem(editMenu, @"Redo", @selector(redo:), @"z",
		        NSEventModifierFlagShift | NSEventModifierFlagCommand, nil);
		[editMenu addItem:[NSMenuItem separatorItem]];
		addItem(editMenu, @"Cut", @selector(cut:), @"x", 0, nil);
		addItem(editMenu, @"Copy", @selector(copy:), @"c", 0, nil);
		addItem(editMenu, @"Paste", @selector(paste:), @"v", 0, nil);
		addItem(editMenu, @"Select All", @selector(selectAll:), @"a", 0, nil);

		NSMenu *windowMenu = addSubmenu(mainMenu, @"Window");
		addItem(windowMenu, @"Minimize", @selector(performMiniaturize:), @"m", 0,
		        nil);
		addItem(windowMenu, @"Zoom", @selector(performZoom:), @"", 0, nil);

		[app setMainMenu:mainMenu];
		[app setWindowsMenu:windowMenu];
		[mainMenu release];
	}
}

#endif // __APPLE__
