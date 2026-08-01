//go:build !windows && !linux && !darwin

package webview

import wv "github.com/chinese-room-solutions/mass-sdk/webview/internal/webview"

type nativeWindow struct {
	wv wv.WebView
}

// Open creates a native webview window. Returns nil if webview is unavailable.
// IconPNG is ignored on these platforms.
func Open(opts Options) WindowInterface {
	w := wv.New(false)
	if w == nil {
		return nil
	}
	w.SetTitle(opts.Title)
	w.SetSize(opts.Width, opts.Height, wv.HintNone)
	w.Navigate(opts.URL)
	return &nativeWindow{wv: w}
}

func (w *nativeWindow) Run()       { w.wv.Run() }
func (w *nativeWindow) Terminate() { w.wv.Terminate() }
func (w *nativeWindow) Destroy()   { w.wv.Destroy() }

// SetTheme is a no-op: these platforms have no themable window frame here.
func (w *nativeWindow) SetTheme(string) {}

// PickFolder is unsupported on these platforms: the app should fall back to a
// manual path input.
func (w *nativeWindow) PickFolder(string) (string, bool, error) { return "", false, ErrUnsupported }

// Screenshot is unsupported on these platforms.
func (w *nativeWindow) Screenshot() ([]byte, error) { return nil, ErrUnsupported }

// Hide, Show, Toggle, and SetOnMinimize are no-ops here: these platforms have
// no tray-fold support, so the window keeps the OS's default minimize/close.
func (w *nativeWindow) Hide()                {}
func (w *nativeWindow) Show()                {}
func (w *nativeWindow) Toggle()              {}
func (w *nativeWindow) SetOnMinimize(func()) {}

// SetOnFileDrop is a no-op: no native drop interception on these platforms.
func (w *nativeWindow) SetOnFileDrop(func(paths []string)) {}

// Eval runs JavaScript in the page on the UI loop.
func (w *nativeWindow) Eval(js string) {
	w.wv.Dispatch(func() { w.wv.Eval(js) })
}
