//go:build windows

package webview

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"syscall"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

// externalNavBinding is the JS-visible name of the Go hand-off used by the
// link shim below.
const externalNavBinding = "__massOpenExternal"

const swShowNormal = 1

// installExternalNavHook keeps the window on the app's own pages: the page
// served from the app's local origin IS the UI, so following an external link
// in-window would replace the app with a remote site and strand the user there
// (a bare webview has no browser chrome to come back with). Links leaving
// originPrefix are handed to the OS default browser instead.
//
// Unlike the Linux and macOS backends this hooks the DOM, not the engine:
// go-webview2 hands out only the webview2.WebView interface, so the
// ICoreWebView2 that owns NavigationStarting/NewWindowRequested is
// unreachable (the binding keeps it in unexported fields and exposes no event
// registration). Init + Bind is the closest equivalent the binding offers; it
// covers link clicks and window.open, which is where external links actually
// come from, but not a navigation started by the page itself or by a server
// redirect.
//
// Must be called before Navigate: the script is injected at document start of
// every page loaded afterwards.
func installExternalNavHook(wv webview2.WebView, originPrefix string) {
	err := wv.Bind(externalNavBinding, func(uri string) {
		// The shim already applied this rule in JS; re-check here so a page
		// can never talk the binding into launching something else.
		if shouldOpenExternally(uri, originPrefix) {
			openExternal(uri)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "webview: binding %s: %v\n", externalNavBinding, err)
		return
	}
	wv.Init(externalNavJS(originPrefix))
}

// externalNavJS mirrors shouldOpenExternally in the page, because the decision
// to cancel a click has to be made synchronously — the binding call is async.
// Go re-applies the rule before anything is opened.
func externalNavJS(originPrefix string) string {
	origin, err := json.Marshal(originPrefix)
	if err != nil {
		panic(err) // a string always marshals
	}
	return `(function () {
	var origin = ` + string(origin) + `;
	function external(href) {
		return typeof href === "string" && href.indexOf(origin) !== 0 &&
			(href.indexOf("http://") === 0 || href.indexOf("https://") === 0 ||
				href.indexOf("mailto:") === 0);
	}
	function anchor(e) {
		var path = e.composedPath ? e.composedPath() : [];
		for (var i = 0; i < path.length; i++) {
			if (path[i] && path[i].tagName === "A" && path[i].href) {
				return path[i];
			}
		}
		return e.target && e.target.closest ? e.target.closest("a[href]") : null;
	}
	function onClick(e) {
		if (e.button === 2 || e.defaultPrevented) {
			return;
		}
		var a = anchor(e);
		if (!a || !external(a.href)) {
			return;
		}
		e.preventDefault();
		window.` + externalNavBinding + `(a.href);
	}
	document.addEventListener("click", onClick);
	document.addEventListener("auxclick", onClick);
	var open = window.open;
	window.open = function (url) {
		var href = "";
		try {
			href = url ? new URL(url, location.href).href : "";
		} catch (err) {
			href = "";
		}
		if (external(href)) {
			window.` + externalNavBinding + `(href);
			return null;
		}
		return open.apply(window, arguments);
	};
})()`
}

// openExternal hands the URI to the OS default handler on its own thread:
// ShellExecuteW may delegate to a shell extension that wants a single-threaded
// apartment, and can block while the browser starts — neither belongs on the
// UI loop the binding calls us from.
func openExternal(uri string) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
			// RPC_E_CHANGED_MODE means COM is already up in another mode on
			// this thread — usable, just don't uninit it.
			if errno, ok := err.(syscall.Errno); !ok || uintptr(errno) != rpcEChangedMode {
				fmt.Fprintf(os.Stderr, "webview: CoInitializeEx: %v\n", err)
				return
			}
		} else {
			defer windows.CoUninitialize()
		}

		verb, err := windows.UTF16PtrFromString("open")
		if err != nil {
			fmt.Fprintf(os.Stderr, "webview: opening %s externally: %v\n", uri, err)
			return
		}
		file, err := windows.UTF16PtrFromString(uri)
		if err != nil {
			fmt.Fprintf(os.Stderr, "webview: opening %s externally: %v\n", uri, err)
			return
		}
		if err := windows.ShellExecute(0, verb, file, nil, nil, swShowNormal); err != nil {
			fmt.Fprintf(os.Stderr, "webview: opening %s externally: %v\n", uri, err)
		}
	}()
}
