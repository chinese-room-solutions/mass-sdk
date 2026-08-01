// Package webview provides a simple cross-platform webview window for MASS
// app standalone UIs. App GUI binaries call Open() to display their UI in a
// native window pointed at their local HTTP server.
//
// Backends: Windows uses jchv/go-webview2 (pure Go, no CGO). Linux and macOS
// use webview/webview_go (CGO), so building a GUI binary on those platforms
// needs CGO_ENABLED=1 and a C toolchain — plus, on Linux, the gtk+-3.0 and
// webkit2gtk-4.1 development headers (the folder picker and screenshot also
// pull in gdk-pixbuf-2.0). PickFolder and Screenshot have native
// implementations on all three OSes; on any other platform they return
// ErrUnsupported.
//
// The window is pinned to the app's own pages: on Linux, link clicks and
// window.open targets that leave the app's origin (http(s) elsewhere, mailto)
// open in the OS default browser instead of replacing the UI — a bare webview
// has no chrome to navigate back with. Windows and macOS don't intercept
// external navigations yet and keep their engines' default behavior.
package webview

import "errors"

// ErrUnsupported is returned by capabilities (e.g. Screenshot) that have no
// implementation on the current platform.
var ErrUnsupported = errors.New("webview: unsupported on this platform")

// WindowInterface represents a native webview window.
type WindowInterface interface {
	// Run starts the event loop and blocks until the window is closed.
	Run()
	// Terminate stops the event loop, causing Run to return on the thread that
	// called it. Safe to call from any goroutine — it posts to the UI loop. Use
	// this to quit from a background goroutine (e.g. a tray "Quit" click); the
	// main thread then calls Destroy after Run returns.
	Terminate()
	// Destroy releases native resources. Must be called after Run returns, on
	// the same thread that called Run.
	Destroy()
	// SetTheme switches the native window chrome (e.g. the title bar) to
	// match "dark" or "light". A no-op on platforms without a themable
	// frame. The page calls this through the bound window.__massSetNativeTheme
	// bridge so a UI theme switch also repaints the title bar.
	SetTheme(theme string)
	// PickFolder opens the OS folder-selection dialog parented to this window
	// and returns the chosen absolute path. ok is false if the user cancelled.
	// Returns ErrUnsupported on platforms without a native picker. Must be
	// called from a goroutine, not the UI event loop.
	PickFolder(title string) (path string, ok bool, err error)
	// Screenshot captures the window's web-content area and returns it as PNG
	// bytes. It lets a local agent see the rendered UI. Returns ErrUnsupported
	// on platforms without a capture implementation. Safe to call from any
	// goroutine.
	Screenshot() (png []byte, err error)
	// Hide hides the window (removing it from the taskbar) while keeping the
	// process and its HTTP backend alive — the "fold to tray" action. Safe to
	// call from any goroutine; it dispatches onto the UI thread.
	Hide()
	// Show re-displays and focuses a hidden or minimized window. Safe to call
	// from any goroutine; it dispatches onto the UI thread.
	Show()
	// Toggle hides the window if it is currently shown, or shows it if hidden.
	// The window tracks its own visibility, so a tray toggle, a left-click Show,
	// and a minimize-to-tray all stay consistent. Safe to call from any
	// goroutine.
	Toggle()
	// SetOnMinimize registers the action to run when the user minimizes the
	// window (the title-bar fold button). Apps that fold to a tray set this to
	// Hide so a minimize tucks the window away instead of leaving it on the
	// taskbar. Closing the window (the X) is unaffected — it still quits.
	// A nil callback (the default) leaves the OS's normal minimize behavior.
	SetOnMinimize(func())
	// SetOnFileDrop registers the action to receive files dropped onto the
	// window from the OS. Only Linux invokes it: WebKitGTK never delivers
	// external file drags to the page's DOM handlers and would instead
	// navigate to the dropped file, so the SDK intercepts them natively and
	// hands the absolute paths here. On Windows and macOS the engines deliver
	// DOM drag events, so apps handle drops in the page and this is a no-op.
	// The callback runs on its own goroutine. In-page (HTML5) drags are
	// unaffected.
	SetOnFileDrop(func(paths []string))
	// Eval runs JavaScript in the page asynchronously, discarding the result.
	// Safe to call from any goroutine; it dispatches onto the UI loop. Use it
	// to push native events (e.g. dropped files' import progress) into page UI.
	Eval(js string)
}

// Options configures the webview window.
type Options struct {
	Title   string
	URL     string
	Width   int
	Height  int
	IconPNG []byte // optional PNG icon data for the window/taskbar
	// Theme is the initial window-chrome theme ("dark" or "light"); empty
	// defaults to dark. SetTheme changes it later.
	Theme string
}
