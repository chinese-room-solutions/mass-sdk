//go:build darwin

package tray

import "fyne.io/systray"

// runTray uses systray's external-loop API on macOS: the status item needs the
// main NSApplication, which the webview already owns and runs. start posts the
// tray into that running main loop; end removes it. (Running systray's own loop
// on a separate thread, as on Windows/Linux, would create a second NSApp and
// fight the webview.)
func runTray(onReady func()) (start, end func()) {
	return systray.RunWithExternalLoop(onReady, nil)
}
