//go:build linux

package webview

/*
#cgo pkg-config: gtk+-3.0

#include <gtk/gtk.h>

// goOnMinimize is the Go callback invoked when the window is iconified.
extern void goOnMinimize(void *window);

// onWindowState is the GTK "window-state-event" handler. When the new state
// includes ICONIFIED (the user clicked minimize), it forwards to Go so the app
// can fold to the tray instead of leaving a minimized window. Returning TRUE
// stops further default handling of the event.
static gboolean onWindowState(GtkWidget *widget, GdkEventWindowState *event, gpointer data) {
	if ((event->changed_mask & GDK_WINDOW_STATE_ICONIFIED) &&
	    (event->new_window_state & GDK_WINDOW_STATE_ICONIFIED)) {
		goOnMinimize((void *)widget);
		return TRUE;
	}
	return FALSE;
}

// installMinimize connects the state handler. Must run on the GTK main thread.
static void installMinimize(void *win) {
	GtkWidget *widget = (GtkWidget *)win;
	if (widget == NULL) {
		return;
	}
	g_signal_connect(widget, "window-state-event", G_CALLBACK(onWindowState), NULL);
}

// hideWindow hides the window without destroying it. Main thread.
static void hideWindow(void *win) {
	GtkWidget *widget = (GtkWidget *)win;
	if (widget != NULL) {
		gtk_widget_hide(widget);
	}
}

// showWindow re-shows and raises the window. Main thread.
//
// The deiconify is load-bearing: a fold hides the window while it is still
// iconified (the minimize that triggered it is never undone), and GTK
// snapshots that state on unmap and re-applies it on the next map. Without
// clearing it, the window would map back minimized, onWindowState would
// immediately fold it again, and a tray click would visibly do nothing.
// gtk_window_present never deiconifies, so it can't cover this itself.
static void showWindow(void *win) {
	GtkWidget *widget = (GtkWidget *)win;
	if (widget != NULL) {
		gtk_window_deiconify(GTK_WINDOW(widget));
		gtk_window_present(GTK_WINDOW(widget));
	}
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

// minimizeCallbacks maps a GtkWindow* to the Go action to run on minimize.
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

// installMinimizeHook connects the GTK state handler so a minimize routes to f.
// Runs on the UI thread (the caller dispatches it).
func installMinimizeHook(handle unsafe.Pointer, f func()) {
	minimizeMu.Lock()
	minimizeCallbacks[handle] = f
	minimizeMu.Unlock()
	C.installMinimize(handle)
}

func hideWindow(handle unsafe.Pointer) { C.hideWindow(handle) }
func showWindow(handle unsafe.Pointer) { C.showWindow(handle) }
