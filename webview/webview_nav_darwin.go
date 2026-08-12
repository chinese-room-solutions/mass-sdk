//go:build darwin

package webview

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

// The MassNavDelegate class and installExternalNav live in
// webview_nav_darwin.m; see the header for why they are a separate
// translation unit.
#include <stdlib.h>

#include "webview_nav_darwin.h"
*/
import "C"

import "unsafe"

//export goShouldOpenExternal
func goShouldOpenExternal(uri, origin *C.char) C.int {
	if shouldOpenExternally(C.GoString(uri), C.GoString(origin)) {
		return 1
	}
	return 0
}

// installExternalNavHook installs the WKNavigationDelegate/WKUIDelegate pair
// so links leaving originPrefix open in the OS browser. Runs on the UI thread
// (the caller dispatches it).
func installExternalNavHook(handle unsafe.Pointer, originPrefix string) {
	cOrigin := C.CString(originPrefix)
	defer C.free(unsafe.Pointer(cOrigin))
	C.installExternalNav(handle, cOrigin)
}
