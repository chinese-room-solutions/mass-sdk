//go:build darwin

package webview

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// pick_folder shows a modal NSOpenPanel in folder-selection mode and returns the
// chosen path as a newly-allocated C string (caller frees), or NULL if the user
// cancelled. parent is the NSWindow*; it is used only to centre the panel. Must
// run on the main thread.
static char *pick_folder(void *parent, const char *title) {
	NSOpenPanel *panel = [NSOpenPanel openPanel];
	[panel setCanChooseFiles:NO];
	[panel setCanChooseDirectories:YES];
	[panel setAllowsMultipleSelection:NO];
	if (title != NULL) {
		[panel setMessage:[NSString stringWithUTF8String:title]];
	}

	if ([panel runModal] != NSModalResponseOK) {
		return NULL;
	}
	NSURL *url = [[panel URLs] firstObject];
	if (url == nil) {
		return NULL;
	}
	const char *path = [[url path] UTF8String];
	if (path == NULL) {
		return NULL;
	}
	return strdup(path);
}
*/
import "C"

import (
	"unsafe"
)

// pickFolder runs NSOpenPanel on the webview's main thread and blocks until the
// user responds. handle is the NSWindow* (unused beyond parenting context).
func pickFolder(handle unsafe.Pointer, dispatch func(func()), title string) (string, bool, error) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	type result struct {
		path string
		ok   bool
	}
	ch := make(chan result, 1)

	dispatch(func() {
		cPath := C.pick_folder(handle, cTitle)
		if cPath == nil {
			ch <- result{}
			return
		}
		defer C.free(unsafe.Pointer(cPath))
		ch <- result{path: C.GoString(cPath), ok: true}
	})

	r := <-ch
	return r.path, r.ok, nil
}
