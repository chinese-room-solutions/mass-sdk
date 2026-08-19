// Package uikit provides reusable HTML-generating helpers for MASS module UIs.
// Modules use Layout() to serve full pages and helper functions for
// Shoelace web components, Tailwind CSS, and Datastar data-binding.
package uikit

import (
	_ "embed"
	"fmt"
	"html"
	"strconv"
)

// ThemeCSS contains the shared MASS theme variables, brand colors, and base
// component styles. Standalone module servers should include this in their
// HTML layout. MASS itself imports the source file in its Tailwind build.
//
//go:embed theme.css
var ThemeCSS string

// StateJS contains the massState sessionStorage API injected into every module page.
// Provides window.massState with get/set/del/clear methods scoped by module name.
//
//go:embed state.js
var StateJS string

// AlertJS contains the massAlert helper — a Shoelace-styled drop-in
// replacement for window.alert. Provides window.massAlert(msg, opts).
//
//go:embed alert.js
var AlertJS string

// ThemeJS contains the massSetTheme helper — a live theme switcher that
// swaps the <html>/<body>/<sl-dialog> classes without a page reload.
//
//go:embed theme.js
var ThemeJS string

// ReloadJS binds F5 and Ctrl/Cmd+R to a reload of the top-level window. A
// webview has no browser chrome to provide them, and a page an app renders
// outside [Layout] needs to include this itself.
//
//go:embed reload.js
var ReloadJS string

// ToInt32 converts a JSON value (string or float64) to int32.
// Datastar sends sl-input type="number" values as JSON strings,
// so this helper handles both representations.
func ToInt32(v any) int32 {
	switch val := v.(type) {
	case float64:
		return int32(val)
	case string:
		n, _ := strconv.ParseInt(val, 10, 32)
		return int32(n)
	default:
		return 0
	}
}

// RenderAlert returns a Shoelace alert HTML fragment with a stable ID.
func RenderAlert(id, msg, variant string, duration int) string {
	if msg == "" {
		return fmt.Sprintf(`<div id="%s"></div>`, html.EscapeString(id))
	}
	dur := ""
	if duration > 0 {
		dur = fmt.Sprintf(` duration="%d"`, duration)
	}
	return fmt.Sprintf(`<div id="%s"><sl-alert variant="%s" open%s>%s</sl-alert></div>`,
		html.EscapeString(id), html.EscapeString(variant), dur, html.EscapeString(msg))
}
