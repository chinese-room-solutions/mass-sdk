//go:build darwin

package webview

import "unsafe"

// installExternalNavHook is a no-op on macOS for now: routing external links
// to the OS browser needs a WKNavigationDelegate policy override this package
// doesn't wire yet, so links keep WebKit's default in-window navigation.
func installExternalNavHook(unsafe.Pointer, string) {}
