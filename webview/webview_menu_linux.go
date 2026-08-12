//go:build linux

package webview

import "unsafe"

// installMainMenu is macOS-only: GTK has no process-wide menu bar, and
// WebKitGTK handles the clipboard and selection shortcuts itself.
func installMainMenu(unsafe.Pointer) {}
