package uikit

import (
	"fmt"
	"runtime"
)

// Layout wraps app content HTML in a full page with all required
// dependencies: Datastar, Shoelace (dark + light themes), the MASS theme
// variables, and a small static utility-class layer (in ThemeCSS) that
// replaces the Tailwind CDN so pages need no runtime CSS compiler. Derive
// theme from the request with ThemeFromRequest (the ?theme= convention) or
// from the app's persisted config. Apps use this in their HTTP handlers to
// serve complete pages for standalone webview windows.
//
// Asset URLs are root-absolute (AssetsPath), so the app must serve
// AssetsHandler on the same origin the page loads from (MountAssets). For
// pages reached through a path-rewriting proxy, use LayoutUnder instead.
func Layout(title, body string, theme Theme) string {
	return LayoutUnder("", title, body, theme)
}

// LayoutUnder is Layout for pages served behind a path-rewriting proxy:
// pathPrefix (e.g. "/mass.llama-cpp") is prepended to every asset URL so the
// browser's asset requests route back through the proxy to this app's
// AssetsHandler, which still mounts at the unprefixed AssetsPath — the proxy
// strips pathPrefix before the request reaches the app's mux.
func LayoutUnder(pathPrefix, title, body string, theme Theme) string {
	info, ok := LookupTheme(string(theme))
	if !ok {
		info, _ = LookupTheme(string(ThemeDark))
	}
	themeClass := theme.HTMLClass()
	bgColor := info.BG
	scheme := string(info.Base)

	themesStyle := ""
	if css := ThemesCSS(); css != "" {
		themesStyle = "<style>" + css + "</style>"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en" class="%[2]s" data-theme="%[14]s" data-os="%[13]s">
<head>
	<meta charset="UTF-8"/>
	<!-- Critical background, first thing in <head>: the external Shoelace
	     stylesheets and module scripts below block first paint until fetched, and
	     the body is held hidden until Shoelace upgrades — so without this the
	     WebView2/browser default white flashes for the whole load. A literal colour
	     (not a CSS var, which theme.css defines later) paints the root immediately. -->
	<style>:root{color-scheme:%[9]s}html{background:%[10]s}</style>
	<meta name="viewport" content="width=device-width, initial-scale=1"/>
	<title>%[3]s</title>
	<link rel="stylesheet" href="%[1]sshoelace/themes/dark.css"/>
	<link rel="stylesheet" href="%[1]sshoelace/themes/light.css"/>
	<!-- Pin Shoelace's base path to our locally-served dist so the autoloader
	     resolves component chunks and icon SVGs from the binary, not a CDN. Must
	     run before the autoloader registers any element. -->
	<script type="module">
		import { setBasePath } from '%[1]sshoelace/utilities/base-path.js';
		setBasePath('%[1]sshoelace');
	</script>
	<script type="module" src="%[1]sshoelace/shoelace-autoloader.js"></script>
	<!-- Load Datastar only after Shoelace has upgraded the form controls it
	     binds to. Shoelace's autoloader registers custom elements
	     asynchronously; if Datastar scans data-bind before sl-input is
	     defined it picks the attribute adapter (reads the static value
	     attribute, not the live .value property) and never recovers, so
	     typed input never reaches the signal. We await whenDefined only for
	     the Shoelace tags actually present on the page (a fixed list would
	     deadlock on a tag an app never uses), with a timeout so a missing
	     upgrade can't hang the load. -->
	<!-- Avoid a flash of unstyled content: the body is hidden until Shoelace has
	     upgraded its custom elements, then revealed. Without this the raw,
	     un-upgraded sl-* markup (icon buttons, dropdowns) paints expanded for a
	     frame on load/reload before snapping into place. The style is inline in
	     <head> so it applies before first paint; a fallback timeout guarantees the
	     body can never stay hidden if an upgrade never resolves. -->
	<style>body{visibility:hidden}body.uikit-ready{visibility:visible}</style>
	<!-- massDatastarReady resolves once Datastar has been imported and applied
	     (or the import failed — a dead promise would hang consumers worse than a
	     degraded page). App boot code that fires Datastar signals or clicks
	     data-on triggers must await it rather than DOMContentLoaded: WebKit fires
	     DOMContentLoaded without waiting for the module below to finish its
	     awaits, so on WebKitGTK/Safari the page becomes "ready" long before
	     data-bind/data-on listeners exist and anything fired at them is lost.
	     Created here, in a classic script that runs during parse, so it exists
	     before the module could ever dispatch. -->
	<script>window.massDatastarReady = new Promise(function (resolve) {
		document.addEventListener('mass:datastar-ready', resolve, { once: true });
	});</script>
	<script type="module">
		const reveal = () => document.body.classList.add('uikit-ready');
		setTimeout(reveal, 2000); // safety: never leave the body hidden.
		const tags = ['sl-input', 'sl-select', 'sl-switch', 'sl-textarea']
			.filter(t => document.querySelector(t));
		await Promise.race([
			Promise.all(tags.map(t => customElements.whenDefined(t))),
			new Promise(r => setTimeout(r, 3000)),
		]);
		try {
			await import('%[1]sdatastar/datastar.js');
		} finally {
			document.dispatchEvent(new Event('mass:datastar-ready'));
		}
		reveal();
	</script>
	<link rel="icon" href="data:,"/>
	<style>%[4]s</style>
	%[11]s
	<script>%[5]s</script>
	<script>%[6]s</script>
	<script>window.__massThemes = %[12]s;</script>
	<script>%[7]s</script>
</head>
<body class="%[2]s min-h-screen">
	%[8]s
</body>
</html>`, pathPrefix+AssetsPath(), themeClass, title, ThemeCSS, StateJS, AlertJS, ThemeJS, body, scheme, bgColor, themesStyle, ThemesJSON(), runtime.GOOS, string(info.Name))
}
