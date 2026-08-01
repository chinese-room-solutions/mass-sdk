//go:build darwin

package webview

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

// capture renders the content view of parent (an NSWindow*) into a PNG written
// to *out (newly malloc'd, caller frees) with length *out_len. Returns 0 on
// success, non-zero on failure. Must run on the main thread.
static int capture(void *parent, unsigned char **out, unsigned long *out_len) {
	NSWindow *window = (NSWindow *)parent;
	if (window == nil) {
		return 1;
	}
	NSView *view = [window contentView];
	if (view == nil) {
		return 2;
	}

	NSRect bounds = [view bounds];
	if (bounds.size.width <= 0 || bounds.size.height <= 0) {
		return 3;
	}

	NSBitmapImageRep *rep = [view bitmapImageRepForCachingDisplayInRect:bounds];
	if (rep == nil) {
		return 4;
	}
	[view cacheDisplayInRect:bounds toBitmapImageRep:rep];

	NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
	if (png == nil) {
		return 5;
	}

	NSUInteger len = [png length];
	unsigned char *copy = malloc(len);
	if (copy == NULL) {
		return 6;
	}
	memcpy(copy, [png bytes], len);
	*out = copy;
	*out_len = (unsigned long)len;
	return 0;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// screenshot captures the window's content view as PNG bytes. handle is the
// NSWindow*. The Cocoa capture runs on the webview's main thread.
func screenshot(handle unsafe.Pointer, dispatch func(func())) ([]byte, error) {
	type result struct {
		png []byte
		err error
	}
	ch := make(chan result, 1)

	dispatch(func() {
		var out *C.uchar
		var outLen C.ulong
		rc := C.capture(handle, &out, &outLen)
		if rc != 0 {
			ch <- result{err: fmt.Errorf("webview: Cocoa screenshot failed (code %d)", int(rc))}
			return
		}
		defer C.free(unsafe.Pointer(out))
		ch <- result{png: C.GoBytes(unsafe.Pointer(out), C.int(outLen))}
	})

	r := <-ch
	return r.png, r.err
}
