//go:build linux || darwin

package webview

import (
	"os"
	"runtime"
	"sync"
	"unsafe"

	wv "github.com/chinese-room-solutions/mass-sdk/webview/internal/webview"
)

// nativeWindow wraps the cross-platform webview_go window and caches the native
// window handle (a GtkWindow* on Linux, an NSWindow* on macOS) for the folder
// picker and screenshot helpers, which live in the per-OS files in this package.
type nativeWindow struct {
	wv         wv.WebView
	handle     unsafe.Pointer
	onMinimize func() // fired when the user minimizes; set via SetOnMinimize

	visMu   sync.Mutex
	visible bool // tracks Show/Hide/minimize so Toggle is consistent
}

// Open creates a native webview window. Returns nil if webview is unavailable.
// IconPNG and Theme (window chrome) are ignored here: on Linux/macOS the window
// frame follows the OS theme and the page controls its own colours.
func Open(opts Options) WindowInterface {
	// WebKitGTK's GL compositor rasterizes text inside async-scroll layers
	// against transparency, which visibly fattens dark-on-light glyphs (list
	// rows read as bold on light themes, then flip to normal on hover when the
	// row paints an opaque background). Software mode renders text uniformly
	// everywhere. Must be set before the web process spawns; an explicit value
	// from the user's environment wins.
	if runtime.GOOS == "linux" {
		if _, set := os.LookupEnv("WEBKIT_DISABLE_COMPOSITING_MODE"); !set {
			if err := os.Setenv("WEBKIT_DISABLE_COMPOSITING_MODE", "1"); err != nil {
				panic(err) // only fails on an invalid key — a programming error
			}
		}
	}
	w := wv.New(false)
	if w == nil {
		return nil
	}
	w.SetTitle(opts.Title)
	w.SetSize(opts.Width, opts.Height, wv.HintNone)
	w.Navigate(opts.URL)
	n := &nativeWindow{wv: w, handle: w.Window(), visible: true}
	// macOS routes the clipboard and selection shortcuts through the app's main
	// menu, which the webview engine never creates. No-op on Linux. Installed
	// before Run() so the menu exists before the window processes any event.
	installMainMenu(n.handle)
	// Keep the window on the app's own pages: navigations leaving the app
	// origin open in the OS browser instead of replacing the UI.
	if origin := originPrefix(opts.URL); origin != "" {
		w.Dispatch(func() { installExternalNavHook(n.handle, origin) })
	}
	return n
}

func (w *nativeWindow) Run()       { w.wv.Run() }
func (w *nativeWindow) Terminate() { w.wv.Terminate() }
func (w *nativeWindow) Destroy()   { w.wv.Destroy() }

// SetTheme is a no-op: these platforms have no separately themable window frame
// here; the page's own CSS drives its appearance.
func (w *nativeWindow) SetTheme(string) {}

// PickFolder shows the native folder chooser parented to this window. ok is
// false when the user cancels.
func (w *nativeWindow) PickFolder(title string) (string, bool, error) {
	return pickFolder(w.handle, w.wv.Dispatch, title)
}

// Screenshot captures the window's content area as PNG bytes.
func (w *nativeWindow) Screenshot() ([]byte, error) {
	return screenshot(w.handle, w.wv.Dispatch)
}

// setVisible records the window's shown/hidden state for Toggle.
func (w *nativeWindow) setVisible(v bool) {
	w.visMu.Lock()
	w.visible = v
	w.visMu.Unlock()
}

func (w *nativeWindow) isVisible() bool {
	w.visMu.Lock()
	defer w.visMu.Unlock()
	return w.visible
}

// Hide hides the window (GtkWindow on Linux, NSWindow on macOS) without
// destroying it; the per-OS helper runs on the UI thread.
func (w *nativeWindow) Hide() {
	w.setVisible(false)
	w.wv.Dispatch(func() { hideWindow(w.handle) })
}

// Show re-displays and focuses a hidden window.
func (w *nativeWindow) Show() {
	w.setVisible(true)
	w.wv.Dispatch(func() { showWindow(w.handle) })
}

// Toggle hides the window if shown, shows it if hidden.
func (w *nativeWindow) Toggle() {
	if w.isVisible() {
		w.Hide()
	} else {
		w.Show()
	}
}

// SetOnMinimize installs the per-OS minimize hook so a fold routes to f instead
// of the default iconify. A nil f leaves default behavior. The window close
// button is never intercepted — it still quits.
func (w *nativeWindow) SetOnMinimize(f func()) {
	w.onMinimize = f
	if f != nil {
		w.wv.Dispatch(func() { installMinimizeHook(w.handle, f) })
	}
}

// SetOnFileDrop installs the native external-file-drop hook (real on Linux; a
// no-op on macOS, where the DOM dropzone receives drops itself).
func (w *nativeWindow) SetOnFileDrop(f func(paths []string)) {
	if f != nil {
		w.wv.Dispatch(func() { installFileDropHook(w.handle, f) })
	}
}

// Eval runs JavaScript in the page. Dispatched onto the UI loop, so it is safe
// from any goroutine.
func (w *nativeWindow) Eval(js string) {
	w.wv.Dispatch(func() { w.wv.Eval(js) })
}
