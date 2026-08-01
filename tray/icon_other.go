//go:build !windows

package tray

// trayIcon passes the PNG through unchanged: the Linux (StatusNotifierItem) and
// macOS trays accept PNG bytes directly. Only Windows needs the ICO wrapper
// (icon_windows.go).
func trayIcon(pngBytes []byte) []byte { return pngBytes }
