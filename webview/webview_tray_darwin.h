// Tray/fold window helpers for the macOS webview backend.
//
// These are implemented in webview_tray_darwin.m as a single Objective-C
// translation unit. The companion cgo file (webview_tray_darwin.go) only
// #includes this header, so the MassMinimizeTarget class is defined exactly
// once: putting the @implementation directly in the cgo preamble made cgo emit
// it into more than one object file, and the macOS linker rejects the duplicate
// Objective-C class/metaclass/ivar symbols.
#ifndef MASS_WEBVIEW_TRAY_DARWIN_H
#define MASS_WEBVIEW_TRAY_DARWIN_H

// goOnMinimize is the Go callback invoked when the user clicks the window's
// minimize button. Defined on the Go side via //export.
extern void goOnMinimize(void *window);

// installMinimize redirects the window's minimize button to goOnMinimize.
// win is an NSWindow*. Must run on the main thread.
void installMinimize(void *win);

// hideWindow orders the window out of the screen list without closing it.
// win is an NSWindow*. Must run on the main thread.
void hideWindow(void *win);

// showWindow brings a hidden window back to front and activates the app.
// win is an NSWindow*. Must run on the main thread.
void showWindow(void *win);

#endif // MASS_WEBVIEW_TRAY_DARWIN_H
