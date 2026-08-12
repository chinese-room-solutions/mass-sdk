// External-navigation hook for the macOS webview backend.
//
// Implemented in webview_nav_darwin.m as a single Objective-C translation
// unit, for the same reason as the tray helpers: defining the delegate class
// in a cgo preamble emits it into more than one object file and the macOS
// linker rejects the duplicate Objective-C class symbols.
#ifndef MASS_WEBVIEW_NAV_DARWIN_H
#define MASS_WEBVIEW_NAV_DARWIN_H

// goShouldOpenExternal applies the shared rule in webview_nav.go: non-zero
// when the URI leaves the app's origin for a scheme the OS should handle.
// Defined on the Go side via //export.
extern int goShouldOpenExternal(char *uri, char *origin);

// installExternalNav makes the window's WKWebView send navigations that leave
// origin to the OS default browser. win is an NSWindow*; origin is copied.
// Must run on the main thread.
void installExternalNav(void *win, const char *origin);

#endif // MASS_WEBVIEW_NAV_DARWIN_H
