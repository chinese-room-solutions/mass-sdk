//go:build darwin

package webview

import "unsafe"

// installFileDropHook is a no-op on macOS: WKWebView delivers external file
// drops to the page's DOM handlers, so the in-page dropzone works without
// native interception.
func installFileDropHook(unsafe.Pointer, func([]string)) {}
