//go:build darwin

package webview

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

// The MassQuitTarget class and installMainMenu live in
// webview_menu_darwin.m; see the header for why they are a separate
// translation unit.
#include "webview_menu_darwin.h"
*/
import "C"

import "unsafe"

// installMainMenu gives the app the standard macOS menu bar. macOS resolves
// Cmd+C/X/V/A/Z inside a WKWebView through the Edit menu's key equivalents, so
// without a main menu those shortcuts do nothing at all. Runs on the UI thread.
func installMainMenu(handle unsafe.Pointer) { C.installMainMenu(handle) }
