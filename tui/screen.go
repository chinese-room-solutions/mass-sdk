package tui

import (
	"io"
	"os"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/term"
)

// stdout is the form's output sink. A package var (not a hard os.Stdout
// reference) so a test can capture frames. An io.Writer, so a term.Phase drawn on
// one of these screens can share it and the two orders can't diverge.
var stdout io.Writer = os.Stdout

// termSizeFn and enterRawModeFn are the terminal probes the engine calls;
// package vars so tests can force a size / a raw-mode refusal without a tty.
var (
	termSizeFn     = termSizeOS
	enterRawModeFn = EnterRawMode
)

// termSize is the current terminal size; falls back to 80x24 when it can't be
// queried.
type termSize struct {
	rows, cols int
}

// altScreen enters the alternate screen buffer + hides the cursor so the form
// draws over a clean canvas and the operator's scrollback is restored on leave.
// Only emits the escapes when styling is active (a real VT); otherwise it is
// inert and the renderer degrades to plain redraws. resizeRows/resizeCols, when
// >0, snap the window to a grid (Windows Terminal opens full-window) — pass 0 to
// skip (Linux/macOS bundles size the terminal at launch).
type altScreen struct {
	active bool
}

// screenDepth refcounts the live altScreens, mirroring the C++ worker's
// term::Screen nesting: the terminal is entered when the count goes 0→1 and
// restored when it goes 1→0, so a wizard can HOLD one session (HoldScreen) while
// its views enter and leave freely inside it. Without an outer hold, every view
// between two others returns the terminal to the PRIMARY screen for a moment —
// and Konsole drapes whatever text selection sits on that screen over the next
// view's content (the "everything looks selected" install phase). Not
// goroutine-safe by design: the whole TUI runs on one goroutine, like the rest
// of this package.
var screenDepth int

func enterAltScreen(resizeRows, resizeCols int) altScreen {
	if !term.StylingEnabled() {
		return altScreen{}
	}
	// Resize the window BEFORE entering the alt screen. Windows Terminal sizes the
	// alt-screen buffer to the window at the moment 1049h runs; resizing afterwards
	// shrinks the visible window but leaves the alt buffer at its original (taller)
	// height, so WT shows a scrollbar over the empty buffer rows below the content.
	// Sizing first means the alt buffer is allocated at the snug height. Applied
	// even for a nested screen — the size is independent of which instance owns
	// the buffer switch, and syncBufferToWindow re-fits the buffer either way.
	if resizeRows > 0 && resizeCols > 0 {
		term.ResizeWindow(resizeRows, resizeCols)
		// CSI 8t resizes the WINDOW, but a freshly launched Windows console keeps a
		// tall screen BUFFER behind it, which shows as a scrollbar over the empty
		// rows below a short form. Match the buffer to the target window size
		// (Windows-only; no-op elsewhere) so there's nothing to scroll. We pass the
		// target explicitly rather than re-querying — CSI 8t is async and a query
		// here can still report the pre-resize size.
		syncBufferToWindow(resizeRows, resizeCols)
	}
	if screenDepth == 0 {
		// Set the terminal's real background (OSC 11) too, not just the per-line
		// OnPage fill — so the margin and scrollbar gutter are indigo as well. The
		// alt screen + hidden cursor give a clean, flicker-free canvas.
		term.SetTerminalBG()
		// Mouse tracking (SGR encoding) for the life of the session: with it on, a
		// drag goes to us as input (discarded by the key reader) instead of creating
		// a terminal selection. Konsole keeps a selection's highlight draped over the
		// alt screen through redraws — an idle drag on the form left the whole
		// install phase looking "selected". Shift+drag still selects, per TUI
		// convention.
		writeRaw("\033[?1049h\033[?25l\033[?1006h\033[?1000h") // alt screen, hide cursor, mouse on
	}
	screenDepth++
	return altScreen{active: true}
}

func (a altScreen) leave() {
	if !a.active {
		return
	}
	if screenDepth > 0 {
		screenDepth--
	}
	if screenDepth > 0 {
		return // an outer hold still owns the terminal
	}
	// Mouse tracking off first (pairing the set in enterAltScreen — a child or
	// the operator's shell must never receive mouse reports), reset SGR so the
	// page bg doesn't bleed onto the restored main screen, then show cursor +
	// leave the alt buffer, and reset the terminal's real background (pairing
	// the OSC 11 set in enterAltScreen) so the operator's terminal isn't left
	// indigo after the TUI exits. The elevation hand-off is the one flow that
	// keeps the themed page across a child process — it opts back in via
	// SuspendScreen, which re-asserts the background.
	writeRaw("\033[?1000l\033[?1006l\033[0m\033[?25h\033[?1049l")
	term.ResetTerminalBG()
}

// HoldScreen opens the wizard-level screen session: one alternate-screen bracket
// that stays put while the views inside it (forms, phases, result modals) come
// and go, exactly like the C++ worker's outer term::Screen. Hold it around a
// whole interactive flow, or the terminal flashes back to the PRIMARY screen
// between views — and Konsole keeps any text selection that sits there draped
// over the next view's content.
//
// The returned release restores the terminal; idempotent, so defer it AND call
// it early before printing anything meant for the operator's own scrollback
// (the exit trace). No-op when styling is off.
func HoldScreen() (release func()) {
	alt := enterAltScreen(0, 0)
	released := false
	return func() {
		if released {
			return
		}
		released = true
		alt.leave()
	}
}

// SuspendScreen hands the terminal to a child process mid-flow (e.g. the sudo
// password prompt of an elevation relaunch): it resets SGR, re-shows the
// cursor, leaves the alt-screen buffer if one is active, and clears to a fresh
// home screen so the caller's notice and the child's inline output land
// cleanly. The themed OSC 11 terminal background is deliberately re-asserted —
// the hand-off is the one flow that keeps the page colour across a child
// process. Follow with ResumeScreen, or with any Run*/Confirm helper (their
// teardown resets the background), so the terminal isn't left themed. No-op
// when styling is off.
func SuspendScreen() {
	if !term.StylingEnabled() {
		return
	}
	writeRaw("\033[?1000l\033[?1006l\033[0m\033[?25h\033[?1049l\033[2J\033[H")
	term.SetTerminalBG()
}

// ResumeScreen returns the terminal to the TUI after a SuspendScreen hand-off:
// back into the alt-screen buffer with the cursor hidden; the next frame render
// paints over whatever the child left. Callers that instead END the flow after
// the child should finish with a Run*/Confirm helper, whose teardown restores
// the terminal. No-op when styling is off.
func ResumeScreen() {
	if !term.StylingEnabled() {
		return
	}
	term.SetTerminalBG()
	writeRaw("\033[?1049h\033[?25l\033[?1006h\033[?1000h")
}

// writeRaw emits straight to stdout. A failed console write has no recovery path
// (stdout IS the UI), so the error is intentionally discarded — the AGENTS
// "genuinely fire-and-forget" exemption.
func writeRaw(s string) {
	_, _ = io.WriteString(stdout, s)
}

// clip truncates a display string to width columns, appending "…" when clipped.
// Byte-oriented (good enough for the ASCII-dominant paths/URLs here).
func clip(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return s[:width-1] + "…"
}

// countLines counts newlines in s (used for vertical centering slack).
func countLines(s string) int { return strings.Count(s, "\n") }
