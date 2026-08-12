package webview

import (
	"net/url"
	"strings"
)

// externalSchemes are the URI schemes handed to the OS when they leave the
// app's origin. Everything else (about:, data:, blob: — in-page mechanics)
// keeps the engine's default handling.
var externalSchemes = []string{"http://", "https://", "mailto:"}

// originPrefix reduces the app URL to its "scheme://host:port/" prefix — the
// boundary the external-navigation hook matches against. Empty (hook skipped)
// when the URL is not a usable web origin.
func originPrefix(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

// shouldOpenExternally reports whether a navigation to uri must leave the app
// window for the OS default handler.
//
// The page served from the app's local origin IS the UI: following an external
// link in-window would replace the app with a remote site and strand the user
// there (a bare webview has no browser chrome to come back with). Only web
// URIs to a foreign origin qualify; same-origin navigations and non-web
// schemes stay in the page.
//
// Every OS hook (WebKitGTK policy decision, WKNavigationDelegate, the WebView2
// link shim) decides with this one function so the three windows behave alike.
func shouldOpenExternally(uri, originPrefix string) bool {
	if uri == "" || strings.HasPrefix(uri, originPrefix) {
		return false
	}
	for _, scheme := range externalSchemes {
		if strings.HasPrefix(uri, scheme) {
			return true
		}
	}
	return false
}
