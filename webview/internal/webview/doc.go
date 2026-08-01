// Package webview is github.com/webview/webview_go at
// v0.0.0-20240831120633-6173450d4dd6 (MIT, see LICENSE + LICENSE.webview),
// vendored for one reason: upstream hardcodes `#cgo pkg-config:
// webkit2gtk-4.0`, and current distros (Fedora 41+, and Debian/Ubuntu are
// following) ship only webkit2gtk-4.1 — the GUI build fails at pkg-config
// before compiling a line. The bundled webview.h already supports 4.1 (it
// resolves the 4.1-vs-4.0 JavaScript API via dlopen at runtime), so the
// only functional change here is that pkg-config line. Windows bits are
// dropped (the SDK uses go-webview2 there). Delete this package and go
// back to the upstream module once it builds against webkit2gtk-4.1.
package webview
