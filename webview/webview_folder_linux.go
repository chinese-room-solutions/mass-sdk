//go:build linux

package webview

/*
#cgo pkg-config: gtk+-3.0

#include <gtk/gtk.h>
#include <stdlib.h>

// pick_folder shows a modal "select folder" dialog parented to parent (a
// GtkWindow*) and returns the chosen path as a newly-allocated C string the
// caller must free, or NULL if the user cancelled. Must run on the GTK main
// thread.
static char *pick_folder(void *parent, const char *title) {
	GtkFileChooserNative *dialog = gtk_file_chooser_native_new(
		title,
		(GtkWindow *)parent,
		GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER,
		"_Open",
		"_Cancel");

	char *result = NULL;
	gint res = gtk_native_dialog_run(GTK_NATIVE_DIALOG(dialog));
	if (res == GTK_RESPONSE_ACCEPT) {
		GtkFileChooser *chooser = GTK_FILE_CHOOSER(dialog);
		char *path = gtk_file_chooser_get_filename(chooser);
		if (path != NULL) {
			result = strdup(path);
			g_free(path);
		}
	}
	g_object_unref(dialog);
	return result;
}
*/
import "C"

import (
	"unsafe"
)

// pickFolder runs the GTK folder chooser on the webview's main thread (dispatch
// hands work to the thread owning the GTK loop) and blocks until the user
// responds. handle is the GtkWindow* parent.
func pickFolder(handle unsafe.Pointer, dispatch func(func()), title string) (string, bool, error) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	type result struct {
		path string
		ok   bool
	}
	ch := make(chan result, 1)

	// gtk_native_dialog_run spins its own nested loop, so it must run on the
	// thread that owns the GTK loop.
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
