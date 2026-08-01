//go:build linux

package webview

/*
#cgo pkg-config: gtk+-3.0 gdk-3.0 gdk-pixbuf-2.0

#include <gtk/gtk.h>
#include <gdk/gdk.h>
#include <gdk-pixbuf/gdk-pixbuf.h>
#include <stdlib.h>
#include <string.h>

// capture grabs the on-screen content of parent (a GtkWindow*) and writes it as
// a PNG into *out (newly g_malloc'd, caller frees with free()); *out_len gets the
// length. Returns 0 on success, non-zero on failure. Must run on the GTK thread.
static int capture(void *parent, unsigned char **out, unsigned long *out_len) {
	GtkWidget *widget = GTK_WIDGET(parent);
	GdkWindow *win = gtk_widget_get_window(widget);
	if (win == NULL) {
		return 1;
	}

	int w = gdk_window_get_width(win);
	int h = gdk_window_get_height(win);
	if (w <= 0 || h <= 0) {
		return 2;
	}

	GdkPixbuf *pixbuf = gdk_pixbuf_get_from_window(win, 0, 0, w, h);
	if (pixbuf == NULL) {
		return 3;
	}

	gchar *buf = NULL;
	gsize len = 0;
	GError *err = NULL;
	gboolean ok = gdk_pixbuf_save_to_buffer(pixbuf, &buf, &len, "png", &err, NULL);
	g_object_unref(pixbuf);
	if (!ok) {
		if (err != NULL) {
			g_error_free(err);
		}
		return 4;
	}

	// Hand the bytes back through a plain malloc'd buffer so the Go side can
	// free() it without linking glib's allocator.
	unsigned char *copy = malloc(len);
	if (copy == NULL) {
		g_free(buf);
		return 5;
	}
	memcpy(copy, buf, len);
	g_free(buf);

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

// screenshot captures the window's rendered content as PNG bytes. handle is the
// GtkWindow*. The GDK capture runs on the webview's main thread.
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
			ch <- result{err: fmt.Errorf("webview: GTK screenshot failed (code %d)", int(rc))}
			return
		}
		defer C.free(unsafe.Pointer(out))
		ch <- result{png: C.GoBytes(unsafe.Pointer(out), C.int(outLen))}
	})

	r := <-ch
	return r.png, r.err
}
