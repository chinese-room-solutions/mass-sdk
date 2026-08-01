package tui

import "github.com/chinese-room-solutions/mass-sdk/term"

// OpenPhase opens the page an action's step list is drawn on: the alternate
// screen, headed by the app's banner centered in the window, with every row the
// phase prints centered under it. cols is the terminal width the page is composed
// for.
//
// The page needs a screen of its own. RunForm leaves its alt screen when the form
// returns, so by the time an action runs we are back on the PRIMARY screen — and a
// page drawn there would clear the operator's own terminal (the command they
// typed, whatever output was on it) out from under them. Everything this page draws
// therefore lives between an enter and a leave, and the terminal comes back exactly
// as the phase found it, background included.
//
// Returned with it is its close: it releases any live transient row and leaves the
// screen. Idempotent — defer it — and call it before a result screen draws, so the
// two screens don't nest.
//
// No screen at all when styling is off: the dumb-terminal fallback keeps its
// question-and-answer transcript and the rows print flush at column 0.
func OpenPhase(art []string, tag string, cols int) (*term.Phase, func()) {
	alt := enterAltScreen(0, 0) // no resize: the form already sized this window
	ph := term.OpenPhasePage(stdout, art, tag, cols)

	closed := false
	return ph, func() {
		if closed {
			return
		}
		closed = true
		ph.Close()
		alt.leave()
	}
}
