// Package uikit provides reusable HTML-generating helpers for MASS-styled UIs.
// Used by MASS itself (model selection dialogs, HF search) and available to
// apps that want to ship a MASS-styled standalone GUI.
package uikit

import "fmt"

// Layout wraps app content HTML in a full page with all required dependencies:
// Datastar, Shoelace (dark + light themes), Tailwind CSS (CDN), and MASS theme variables.
//
// theme should be "dark" or "light". Apps use this in their HTTP handlers
// to serve complete pages for standalone webview windows.
func Layout(title, body, theme string) string {
	themeClass := "sl-theme-dark"
	if theme == "light" {
		themeClass = "sl-theme-light"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en" class="%s">
<head>
	<meta charset="UTF-8"/>
	<meta name="viewport" content="width=device-width, initial-scale=1"/>
	<title>%s</title>
	<script type="module" src="https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.0-RC.7/bundles/datastar.js"></script>
	<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@shoelace-style/shoelace@2.20.1/cdn/themes/dark.css"/>
	<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@shoelace-style/shoelace@2.20.1/cdn/themes/light.css"/>
	<script type="module" src="https://cdn.jsdelivr.net/npm/@shoelace-style/shoelace@2.20.1/cdn/shoelace-autoloader.js"></script>
	<script src="https://cdn.tailwindcss.com"></script>
	<link rel="icon" href="data:,"/>
	<style>%s</style>
	<script>%s</script>
	<script>%s</script>
</head>
<body class="%s min-h-screen">
	%s
</body>
</html>`, themeClass, title, ThemeCSS, StateJS, AlertJS, themeClass, body)
}
