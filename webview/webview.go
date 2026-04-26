// Package webview provides a simple cross-platform webview window for MASS
// app standalone UIs. App GUI binaries call Open() to display their UI in a
// native window pointed at their local HTTP server.
package webview

// WindowInterface represents a native webview window.
type WindowInterface interface {
	// Run starts the event loop and blocks until the window is closed.
	Run()
	// Destroy releases native resources. Must be called after Run returns.
	Destroy()
}

// Options configures the webview window.
type Options struct {
	Title   string
	URL     string
	Width   int
	Height  int
	IconPNG []byte // optional PNG icon data for the window/taskbar
}
