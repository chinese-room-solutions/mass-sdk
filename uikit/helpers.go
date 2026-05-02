// Package uikit provides reusable HTML-generating helpers for MASS module UIs.
// Modules use Layout() to serve full pages and helper functions for
// Shoelace web components, Tailwind CSS, and Datastar data-binding.
package uikit

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
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

// FilenameToDlID converts a filename to a stable HTML element ID for download progress.
func FilenameToDlID(filename string) string {
	var b strings.Builder
	b.WriteString("dl-")
	for _, r := range filename {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
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

// RenderConfigStatus returns an HTML fragment for the config save status area.
func RenderConfigStatus(msg string, isError bool) string {
	if msg == "" {
		return `<div id="pe-config-status"></div>`
	}
	variant := "success"
	if isError {
		variant = "danger"
	}
	return RenderAlert("pe-config-status", msg, variant, 3000)
}

// MergeSignalsIntoConfig overrides a ModelConfigData with values from
// MASS's gui-config signals, so the module UI shows persisted values.
func MergeSignalsIntoConfig(cfg ModelConfigData, signals json.RawMessage) ModelConfigData {
	var s struct {
		ModelPath     string `json:"modelpath"`
		ContextSize   any    `json:"contextsize"`
		Threads       any    `json:"threads"`
		GpuLayers     any    `json:"gpulayers"`
		MaxConcurrent any    `json:"maxconcurrent"`
		MainGpu       string `json:"maingpu"`
		TensorSplit   string `json:"tensorsplit"`
	}
	if err := json.Unmarshal(signals, &s); err != nil {
		return cfg
	}
	if s.ModelPath != "" {
		cfg.Path = s.ModelPath
	}
	if v := ToInt32(s.ContextSize); v > 0 {
		cfg.ContextSize = v
	}
	cfg.Threads = ToInt32(s.Threads)
	cfg.GpuLayers = ToInt32(s.GpuLayers)
	if v := ToInt32(s.MaxConcurrent); v > 0 {
		cfg.MaxConcurrent = v
	}
	if s.MainGpu != "" {
		cfg.MainGPU = s.MainGpu
	}
	if s.TensorSplit != "" {
		cfg.TensorSplit = s.TensorSplit
	}
	return cfg
}
