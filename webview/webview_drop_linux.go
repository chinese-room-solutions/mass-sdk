//go:build linux

package webview

/*
#cgo pkg-config: gtk+-3.0

#include <gtk/gtk.h>
#include <string.h>

// goOnFileDrop is the Go callback invoked with the dropped files' paths,
// joined by '\n'. The window pointer keys the per-window callback map.
extern void goOnFileDrop(void *window, char *paths);

// WebKitGTK handles external file drags natively: it never delivers them to
// the page's DOM handlers and its default action is to NAVIGATE to the
// dropped file, replacing the app UI with WebKit's document viewer. These
// handlers claim external drags on the webview widget before WebKit's class
// handlers run, so the app receives file paths instead. Internal drags
// (HTML5 drag-and-drop within the page — the drag source widget is the
// webview itself) are left alone by returning FALSE, which lets emission
// continue to WebKit.

// externalUriDrag reports whether this drag comes from outside the app and
// carries a file list.
static gboolean externalUriDrag(GtkWidget *widget, GdkDragContext *context) {
	if (gtk_drag_get_source_widget(context) != NULL) {
		return FALSE; // in-app drag (note/folder move)
	}
	GList *targets = gdk_drag_context_list_targets(context);
	for (GList *l = targets; l != NULL; l = l->next) {
		gchar *name = gdk_atom_name(GDK_POINTER_TO_ATOM(l->data));
		gboolean match = (name != NULL && strcmp(name, "text/uri-list") == 0);
		g_free(name);
		if (match) {
			return TRUE;
		}
	}
	return FALSE;
}

static gboolean onDragMotion(GtkWidget *widget, GdkDragContext *context,
                             gint x, gint y, guint time, gpointer window) {
	if (!externalUriDrag(widget, context)) {
		return FALSE;
	}
	gdk_drag_status(context, GDK_ACTION_COPY, time);
	return TRUE;
}

static gboolean onDragDrop(GtkWidget *widget, GdkDragContext *context,
                           gint x, gint y, guint time, gpointer window) {
	if (!externalUriDrag(widget, context)) {
		return FALSE;
	}
	gtk_drag_get_data(widget, context, gdk_atom_intern("text/uri-list", FALSE), time);
	return TRUE;
}

static void onDragDataReceived(GtkWidget *widget, GdkDragContext *context,
                               gint x, gint y, GtkSelectionData *data,
                               guint info, guint time, gpointer window) {
	if (!externalUriDrag(widget, context)) {
		return;
	}
	// Stop WebKit's class handler from also processing (= navigating).
	g_signal_stop_emission_by_name(widget, "drag-data-received");

	gchar **uris = gtk_selection_data_get_uris(data);
	GString *joined = g_string_new(NULL);
	if (uris != NULL) {
		for (gchar **u = uris; *u != NULL; u++) {
			gchar *path = g_filename_from_uri(*u, NULL, NULL);
			if (path == NULL) {
				continue; // non-file URI (e.g. a remote link) — skip
			}
			if (joined->len > 0) {
				g_string_append_c(joined, '\n');
			}
			g_string_append(joined, path);
			g_free(path);
		}
		g_strfreev(uris);
	}
	if (joined->len > 0) {
		goOnFileDrop(window, joined->str);
	}
	g_string_free(joined, TRUE);
	gtk_drag_finish(context, TRUE, FALSE, time);
}

// installFileDrop connects the drag handlers on the window's webview child
// (the actual GTK drag destination). Must run on the GTK main thread.
static void installFileDrop(void *win) {
	GtkWidget *window = (GtkWidget *)win;
	if (window == NULL) {
		return;
	}
	GtkWidget *target = gtk_bin_get_child(GTK_BIN(window));
	if (target == NULL) {
		target = window;
	}
	g_signal_connect(target, "drag-motion", G_CALLBACK(onDragMotion), win);
	g_signal_connect(target, "drag-drop", G_CALLBACK(onDragDrop), win);
	g_signal_connect(target, "drag-data-received", G_CALLBACK(onDragDataReceived), win);
}
*/
import "C"

import (
	"strings"
	"sync"
	"unsafe"
)

// dropCallbacks maps a GtkWindow* to the Go action receiving dropped paths.
var (
	dropMu        sync.Mutex
	dropCallbacks = map[unsafe.Pointer]func([]string){}
)

//export goOnFileDrop
func goOnFileDrop(window unsafe.Pointer, paths *C.char) {
	dropMu.Lock()
	f := dropCallbacks[window]
	dropMu.Unlock()
	if f == nil {
		return
	}
	list := strings.Split(C.GoString(paths), "\n")
	// The callback likely does I/O (reading the files); keep the GTK main
	// thread free so the window stays responsive.
	go f(list)
}

// installFileDropHook connects the GTK drag handlers so external file drops
// route to f. Runs on the UI thread (the caller dispatches it).
func installFileDropHook(handle unsafe.Pointer, f func([]string)) {
	dropMu.Lock()
	dropCallbacks[handle] = f
	dropMu.Unlock()
	C.installFileDrop(handle)
}
