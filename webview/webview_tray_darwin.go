//go:build darwin

package webview

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

// The Objective-C MassMinimizeTarget class and the install/hide/show helpers
// live in webview_tray_darwin.m, which cgo compiles as a single translation
// unit. Defining the @implementation inline here instead made cgo emit the
// class into more than one object file, and the macOS linker rejected the
// duplicate _OBJC_CLASS_$/_OBJC_METACLASS_$/_OBJC_IVAR_$ symbols.
#include "webview_tray_darwin.h"
*/
import "C"

import (
	"sync"
	"unsafe"
)

// minimizeCallbacks maps an NSWindow* to the Go action to run when its minimize
// button is clicked. Keyed by the pointer value since the C callback only has
// the window pointer.
var (
	minimizeMu        sync.Mutex
	minimizeCallbacks = map[unsafe.Pointer]func(){}
)

//export goOnMinimize
func goOnMinimize(window unsafe.Pointer) {
	minimizeMu.Lock()
	f := minimizeCallbacks[window]
	minimizeMu.Unlock()
	if f != nil {
		f()
	}
}

// installMinimizeHook redirects the window's minimize button to f. Runs on the
// UI thread (the caller dispatches it).
func installMinimizeHook(handle unsafe.Pointer, f func()) {
	minimizeMu.Lock()
	minimizeCallbacks[handle] = f
	minimizeMu.Unlock()
	C.installMinimize(handle)
}

func hideWindow(handle unsafe.Pointer) { C.hideWindow(handle) }
func showWindow(handle unsafe.Pointer) { C.showWindow(handle) }
