// Package term is a dependency-free terminal styling layer for interactive
// console UIs — the one place an app talks to a human at a console rather than a
// log. ANSI SGR escapes are emitted ONLY when stdout is a real terminal that can
// be put into a VT-processing mode; otherwise every helper returns its text
// unstyled, so piped/redirected output and dumb terminals stay clean. NO_COLOR
// (https://no-color.org) disables colour even on a TTY.
//
// This is a Go port of the worker's term.cpp/term.hpp (mass-worker-llama-cpp);
// the C++ stays there as the reference, this is canonical for the Go apps (MASS,
// Grimoire). Presentation for human prompts, not diagnostics — it lives outside
// the structured-logging path on purpose.
package term

import (
	"os"
	"sync"
)

// getEnv returns the value of an environment variable, or ("", false) when it is
// unset or empty (the two are treated the same, matching the worker).
func getEnv(name string) (string, bool) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

var (
	stylingOnce    sync.Once
	stylingResult  bool
	truecolorOnce  sync.Once
	truecolorState bool

	// Test override: when non-nil, short-circuits the cached probes so the pure
	// transforms can be exercised in both the styled and unstyled paths without a
	// real TTY. nil in production. Set via ForceCaps.
	forceStyling   *bool
	forceTruecolor *bool
)

// ForceCaps overrides the styling/truecolor probes so rendering is
// deterministic regardless of the host terminal — for tests (this package's
// and dependents': the tui golden fixtures render with styling forced off).
// Returns a restore func that re-enables the real probes. Not a production
// switch.
func ForceCaps(styling, truecolor bool) (restore func()) {
	forceStyling, forceTruecolor = &styling, &truecolor
	return func() { forceStyling, forceTruecolor = nil, nil }
}

// StylingEnabled resolves once whether styling is active for this process:
// stdout is a terminal, NO_COLOR is unset, and (on Windows) VT processing could
// be enabled. Safe to call repeatedly; the first call does the detection/enable
// and the result is cached thereafter.
func StylingEnabled() bool {
	if forceStyling != nil {
		return *forceStyling
	}
	stylingOnce.Do(func() { stylingResult = detectStyling() })
	return stylingResult
}

// TruecolorEnabled reports whether the terminal advertises 24-bit colour.
// Gradients use this; everything else falls back to the 16-colour helpers. False
// (→ 16-colour fallback) when styling is off.
//
// COLORTERM=truecolor|24bit is the portable signal most Unix terminals set
// (Konsole, GNOME Terminal, iTerm, …). Windows consoles generally do NOT set it
// even when they fully support truecolor, so relying on COLORTERM alone wrongly
// downgrades Windows Terminal to a muddy 16-colour approximation. On Windows we
// additionally trust two reliable signals:
//   - WT_SESSION is set  → Windows Terminal (24-bit since day one)
//   - StylingEnabled()   → on Windows this is true ONLY when we successfully
//     turned on ENABLE_VIRTUAL_TERMINAL_PROCESSING, which modern conhost/WT
//     accept (Win10 1703+) and which implies truecolor support.
func TruecolorEnabled() bool {
	if forceTruecolor != nil {
		return *forceTruecolor
	}
	truecolorOnce.Do(func() { truecolorState = detectTruecolor() })
	return truecolorState
}

func detectTruecolor() bool {
	if !StylingEnabled() {
		return false
	}
	if ct, ok := getEnv("COLORTERM"); ok && (ct == "truecolor" || ct == "24bit") {
		return true
	}
	return detectTruecolorOS()
}
