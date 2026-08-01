package term

import (
	"io"
	"strings"
)

// A TRANSIENT ROW is a terminal row a renderer rewrites in place — a spinner
// frame, a progress bar — and that carries no newline of its own. Only one may be
// live at a time, and it has to be gone before anything else writes: an appended
// write lands on the same row and reads as garbage (a "22/22" bar with a step
// line smeared over it is the bug this rule exists for). Phase owns that row, and
// every method that prints ends it first, so a step line always starts clean.

// Phase renders the step list of an action — the transcript between a wizard's
// form and its result screen. Two shapes:
//
//   - OpenPhasePage: the FORM path. Clears to a fresh page headed by the app's
//     banner, with every row centered in the window under it.
//   - NewPhase: the scripted face (--install/--uninstall) and the dumb-terminal
//     fallback. No page and no centering — that output goes to a log, or sits under
//     a question-and-answer transcript that starts at column 0.
//
// The centering belongs to the page that drew the banner the rows sit under, so the
// window width is per-Phase state rather than a column read from the live window: a
// run that opens no page prints flush left. Not goroutine-safe; drive it from one
// goroutine.
type Phase struct {
	out       io.Writer
	cols      int
	transient bool
}

// progressBarWidth is the cell width of the bar Progress draws.
const progressBarWidth = 24

// NewPhase returns a Phase that prints flush at column 0 and draws no page of its
// own.
func NewPhase(out io.Writer) *Phase { return &Phase{out: out} }

// OpenPhasePage clears the screen to the page background, prints art+tag as a
// banner centered on the window axis, and returns a Phase that centers every row it
// prints in that same cols-wide window. cols is the terminal width.
//
// THE CALLER OWNS THE SCREEN THIS CLEARS, and it must not be the operator's own:
// on the primary screen the clear erases their command and whatever output was
// there. Use tui.OpenPhase, which hosts the page on the alternate screen and
// restores it on close.
//
// Form path only: on a non-styling stream it degrades to NewPhase — no clear (it
// would wipe a dumb terminal's transcript, or spray escapes into a log) and no
// centering.
func OpenPhasePage(out io.Writer, art []string, tag string, cols int) *Phase {
	p := &Phase{out: out}
	if !StylingEnabled() {
		return p
	}
	p.cols = cols
	p.print(PageBG() + "\033[2J\033[H" + Banner(art, tag, cols))
	return p
}

// Close releases the phase's row: a spinner frame or progress bar still live (an
// early return) is erased, so whatever prints next — the result screen, the exit
// summary — starts on a clean row. Idempotent; defer it.
func (p *Phase) Close() { p.endTransient() }

// Heading prints a section heading (blank row, title, rule) centered on the page.
// The title and its rule are the same width, so centering each on its own keeps the
// two aligned with each other.
func (p *Phase) Heading(title string) {
	p.endTransient()
	p.print(p.center(Heading(title)))
}

// Line prints one step row centered on the page. Compose the status glyph in front
// of the text with OKMark/FailMark/NoteMark.
//
// Centered per row by its own width — the list has no left edge of its own, every
// row just sits in the middle of the window like the banner above it.
func (p *Phase) Line(text string) {
	p.endTransient()
	p.print(p.center(text) + "\n")
}

// Progress redraws a determinate progress bar in place, centered on its own width.
// The row stays live until the next print replaces it, so the step's ✔/✖ lands where
// the bar was. Draws nothing when styling is off — a pipe or a log gets the step rows
// instead of a smear of carriage returns.
func (p *Phase) Progress(done, total int) {
	if !StylingEnabled() {
		return
	}
	p.transientRow(ProgressBar(done, total, progressBarWidth))
}

// center centers every non-empty row of block on the page's width. Blank rows stay
// blank (a padded one would carry trailing whitespace, and a block that ends in a
// newline has to leave the cursor at column 0), and a phase with no page — cols 0 —
// gets the block back unchanged, flush left.
func (p *Phase) center(block string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = Center(line, p.cols)
		}
	}
	return strings.Join(lines, "\n")
}

// transientRow rewrites the phase's live row in place: back to column 0, ERASE, then
// the row centered on the page's width — no newline, so the row stays live. The erase
// is what makes a centered transient row safe to redraw: a bar whose pad shifted (a
// wider count, "9/22" → "10/22") would otherwise leave the old row's tail behind.
func (p *Phase) transientRow(row string) {
	p.print("\r\033[K" + Center(row, p.cols))
	p.transient = true
}

// endTransient erases the live transient row, leaving the cursor at column 0 of a
// now-blank line so the next write starts clean. No-op when nothing is live.
func (p *Phase) endTransient() {
	if !p.transient {
		return
	}
	p.transient = false
	p.print("\r\033[K")
}

// print writes to the phase's sink. A failed console write has no recovery path
// (the console IS the output), so the error is intentionally discarded — the
// AGENTS "genuinely fire-and-forget" exemption.
func (p *Phase) print(s string) { _, _ = io.WriteString(p.out, s) }
