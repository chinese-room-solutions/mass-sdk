//go:build linux

package webview

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1

#include <stdlib.h>

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

// goOpenExternal receives a URI to open in the OS default handler. The string
// is only valid for the duration of the call (the Go side copies it).
extern void goOpenExternal(char *uri);

// The page served from the app's local origin IS the UI: following an
// external link in-window would replace the app with a remote site and
// strand the user there (a bare webview has no browser chrome to come back
// with). onDecidePolicy sends web navigations that leave the app's origin to
// the OS browser instead, covering both in-place navigations (target=_self)
// and new-window requests (target=_blank, which a bare webview would
// otherwise silently drop).
//
// Only http(s) to a foreign origin and mailto are redirected. Same-origin
// navigations and non-web schemes (about:, data:, blob: — in-page mechanics)
// keep WebKit's default handling.
static gboolean onDecidePolicy(WebKitWebView *web_view,
                               WebKitPolicyDecision *decision,
                               WebKitPolicyDecisionType type,
                               gpointer origin) {
	if (type != WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION &&
	    type != WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION) {
		return FALSE; // response decisions etc.: default handling
	}
	WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(
		WEBKIT_NAVIGATION_POLICY_DECISION(decision));
	WebKitURIRequest *req = webkit_navigation_action_get_request(action);
	const gchar *uri = req != NULL ? webkit_uri_request_get_uri(req) : NULL;
	if (uri == NULL || g_str_has_prefix(uri, (const gchar *)origin)) {
		return FALSE;
	}
	gboolean external = g_str_has_prefix(uri, "http://") ||
	                    g_str_has_prefix(uri, "https://") ||
	                    g_str_has_prefix(uri, "mailto:");
	if (!external) {
		return FALSE;
	}
	webkit_policy_decision_ignore(decision);
	goOpenExternal((char *)uri);
	return TRUE;
}

// installExternalNav connects the policy handler on the window's webview
// child. origin is copied once and lives as long as the window. Must run on
// the GTK main thread.
static void installExternalNav(void *win, const char *origin) {
	GtkWidget *window = (GtkWidget *)win;
	if (window == NULL) {
		return;
	}
	GtkWidget *child = gtk_bin_get_child(GTK_BIN(window));
	if (child == NULL || !WEBKIT_IS_WEB_VIEW(child)) {
		return;
	}
	g_signal_connect(child, "decide-policy", G_CALLBACK(onDecidePolicy), g_strdup(origin));
}
*/
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"unsafe"
)

//export goOpenExternal
func goOpenExternal(uri *C.char) {
	u := C.GoString(uri)
	// Off the GTK main thread; Run (Start+Wait) also reaps the child —
	// xdg-open exits as soon as it hands the URI to the real handler.
	go func() {
		if err := exec.Command("xdg-open", u).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "webview: opening %s externally: %v\n", u, err)
		}
	}()
}

// installExternalNavHook connects the WebKitGTK navigation-policy handler so
// links leaving originPrefix open in the OS browser. Runs on the UI thread
// (the caller dispatches it).
func installExternalNavHook(handle unsafe.Pointer, originPrefix string) {
	cOrigin := C.CString(originPrefix)
	defer C.free(unsafe.Pointer(cOrigin))
	C.installExternalNav(handle, cOrigin)
}
