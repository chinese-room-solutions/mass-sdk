//go:build !windows

package webview

import wv "github.com/webview/webview_go"

type nativeWindow struct {
	wv wv.WebView
}

// Open creates a native webview window. Returns nil if webview is unavailable.
// IconPNG is ignored on non-Windows platforms.
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

func (w *nativeWindow) Run()     { w.wv.Run() }
func (w *nativeWindow) Destroy() { w.wv.Destroy() }
