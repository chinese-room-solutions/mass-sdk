package term

import (
	"fmt"
	"os"
	"strings"
)

// sgr wraps text in an SGR code + reset, or returns it unchanged when styling is
// off.
func sgr(code, text string) string {
	if !StylingEnabled() {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

// rgbChannel is a 24-bit colour.
type rgbColor struct{ r, g, b uint8 }

// RGB wraps text in a 24-bit foreground colour when truecolor is available, the
// nearest basic ANSI colour when only 16-colour styling is on, or unchanged when
// styling is off. Lets gradient code emit per-character colours that still
// degrade sensibly.
func RGB(r, g, b uint8, text string) string {
	if !StylingEnabled() {
		return text
	}
	if TruecolorEnabled() {
		return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r, g, b, text)
	}
	return sgr(nearestANSIFG(r, g, b), text) // 16-colour fallback
}

// nearestANSIFG maps a 24-bit colour to the nearest basic ANSI foreground code
// (30-37) for the 16-colour fallback: pick the dominant channel(s) by a coarse
// threshold. Good enough for the blue→cyan/synthwave ramps, which collapse to
// cyan/blue/magenta.
func nearestANSIFG(r, g, b uint8) string {
	R, G, B := r > 0x7f, g > 0x7f, b > 0x7f
	switch {
	case R && B && !G:
		return "35" // magenta (synthwave end of the ramp)
	case B && G && !R:
		return "36" // cyan
	case B && !G && !R:
		return "34" // blue
	case G && !B && !R:
		return "32" // green
	case R && G && !B:
		return "33" // yellow
	case R && !G && !B:
		return "31" // red
	case R && G && B:
		return "37" // white
	default:
		return "36" // default toward the cyan base
	}
}

// --- Style wrappers ---------------------------------------------------------

func Bold(text string) string    { return sgr("1", text) }
func Dim(text string) string     { return sgr("2", text) }
func Blue(text string) string    { return sgr("34", text) }
func Magenta(text string) string { return sgr("35", text) }
func Green(text string) string   { return sgr("32", text) }
func Yellow(text string) string  { return sgr("33", text) }
func Red(text string) string     { return sgr("31", text) }

// Muted is a muted purple for subordinate chrome (help/footer lines): the
// selection band's hue lifted to a legible foreground value, so hints read as a
// hint, not content.
func Muted(text string) string { return RGB(150, 120, 200, text) }

// Cyan is a TRUE blue-cyan via truecolor (0,240,255) so it never renders as a
// terminal palette's green-ish ANSI "36". Falls back to ANSI 36 when truecolor
// is unavailable. The UI's cool base.
func Cyan(text string) string { return RGB(0, 240, 255, text) }

// Accent is the synthwave neon-pink (255,60,180) — the warm counterpart to the
// cyan grid, used sparingly for choice brackets, the • note glyph, and the
// status line.
func Accent(text string) string {
	if !StylingEnabled() {
		return text
	}
	return RGB(255, 60, 180, text)
}

// --- Gradients --------------------------------------------------------------

// sunsetRGB samples the synthwave "sunset" ramp at t in [0,1]: hot magenta →
// coral → gold. A two-segment linear interpolation. Shared by the banner
// wordmark and the selected-row bar so the whole UI reads as one sunset.
func sunsetRGB(t float64) rgbColor {
	magenta := rgbColor{255, 40, 160}
	coral := rgbColor{255, 110, 70}
	gold := rgbColor{255, 210, 90}
	if t < 0.5 {
		return blend(magenta, coral, t*2.0)
	}
	return blend(coral, gold, (t-0.5)*2.0)
}

// coolRGB samples the cool "grid" ramp at t in [0,1]: electric cyan (0,240,255)
// → deep blue (70,110,235). The counterpart to the sunset, for unselected rows.
func coolRGB(t float64) rgbColor {
	return rgbColor{lerp(0, 70, t), lerp(240, 110, t), lerp(255, 235, t)}
}

func lerp(a, b uint8, u float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*u)
}

func blend(a, b rgbColor, u float64) rgbColor {
	return rgbColor{lerp(a.r, b.r, u), lerp(a.g, b.g, u), lerp(a.b, b.b, u)}
}

// CoolGradient paints text with the cool grid gradient (electric cyan → deep
// blue). width is the ramp's reference width so several pieces can share one
// left→right ramp (pass the widest line's width); start is the column the text
// begins at, so a label and the value after it continue the SAME ramp instead of
// each restarting at cyan. Degrades to flat per-char colour in 16-colour and to
// plain text when styling is off.
func CoolGradient(text string, width, start int) string {
	if !StylingEnabled() {
		return text
	}
	var b strings.Builder
	col := start
	for _, run := range text {
		t := 0.0
		if width > 1 {
			t = float64(col) / float64(width-1)
		}
		c := coolRGB(t)
		b.WriteString(RGB(c.r, c.g, c.b, string(run)))
		col++
	}
	return b.String()
}

// Cool paints a standalone line with the cool grid gradient (electric cyan →
// deep blue) ramped over its own length — the convenience form of CoolGradient
// for a self-contained string (a result line, a prompt) rather than a piece
// sharing a wider ramp. VisibleWidth so any nested SGR doesn't skew the ramp.
func Cool(text string) string { return CoolGradient(text, VisibleWidth(text), 0) }

// Heading is a section heading: a blank line, a bold-cyan title, and an
// underline rule the width of the title (dimmed). Ready-to-print block.
func Heading(title string) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(Bold(Cyan(title)))
	b.WriteString("\n")
	b.WriteString(Dim(strings.Repeat("-", len([]rune(title)))))
	b.WriteString("\n")
	return b.String()
}

// --- Layout -----------------------------------------------------------------

// TerminalWidth returns the terminal width in columns, or 0 when it can't be
// determined (not a TTY / query failed). Callers that center on it should treat
// 0 as "don't center".
func TerminalWidth() int { return terminalWidthOS() }

// VisibleWidth returns the number of display columns text occupies: runes minus
// ANSI SGR escapes (which take no space). UTF-8 multibyte counts as one column.
// Approximate (assumes 1 column per codepoint — fine for Latin/box glyphs), but
// correct in the presence of the colour codes the helpers emit.
func VisibleWidth(text string) int {
	cols := 0
	rs := []rune(text)
	for i := 0; i < len(rs); {
		if rs[i] == 0x1b { // skip an ANSI escape: ESC [ ... <final letter>
			if i+1 < len(rs) && rs[i+1] == '[' {
				i += 2
			} else {
				i++
			}
			for i < len(rs) {
				f := rs[i]
				i++
				if (f >= 'A' && f <= 'Z') || (f >= 'a' && f <= 'z') {
					break
				}
			}
			continue
		}
		cols++
		i++
	}
	return cols
}

// Center left-pads one line with spaces so its visible content is centered
// within width columns. Returns the line unchanged when styling is off, width is
// 0, or the content is already wider than width. Operates on VisibleWidth, so a
// colour-wrapped or box-art line centers by what's actually shown.
func Center(line string, width int) string {
	if !StylingEnabled() || width <= 0 {
		return line
	}
	w := VisibleWidth(line)
	if w >= width {
		return line
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + line
}

// ContentWidth is the reading band every full-screen view composes within: a cap
// on the centring axis, so a very wide window doesn't stretch a line of prose (or
// strand a centered banner far from the body under it).
const ContentWidth = 92

// Gutter is the column that separates a label from its value. The wizard's field
// grid (tui.MenuLayout.Gap) and a Summary's rows are the same two-column table seen
// twice — form, then result — so they read as one only while they share this gap.
const Gutter = 12

// ContentOrigin returns the column a ContentWidth band starts at when centered in
// a cols-wide window: 0 when the window is no wider than the band, and 0 for
// cols<=0 (unknown width → don't center).
//
// A full-screen view composes its block at min(cols, ContentWidth) and then
// shifts the whole composed block here with IndentBlock. So the view keeps its own
// body's centring axis AND the band sits in the middle of the window instead of
// hugging the left edge.
func ContentOrigin(cols int) int {
	if cols <= ContentWidth {
		return 0
	}
	return (cols - ContentWidth) / 2
}

// IndentBlock shifts a whole composed block right by n columns: every NON-EMPTY
// line gets n leading spaces. The block-level companion to Center (see
// ContentOrigin), gated the same way — n<=0 or styling off returns block
// unchanged, so piped and captured output stays flush left.
//
// Empty lines stay empty on purpose: a block ending in a newline must leave the
// cursor at column 0 (an installer's elevation notice is followed by sudo's own
// password prompt), and blank rows carry no trailing whitespace.
func IndentBlock(block string, n int) string {
	if !StylingEnabled() || n <= 0 {
		return block
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

// Wrap word-wraps PLAIN text to at most width VISIBLE columns per line, breaking
// on spaces (a single word longer than width is left overlong rather than split).
// Widths are measured with VisibleWidth, so a multibyte-but-single-column glyph
// (e.g. the "·" separator these UIs use) counts as one column, not its byte
// length. Any '\n' already in text forces a hard break, so a caller can put each
// sentence on its own line by joining them with '\n' and letting this wrap each.
// Returns the resulting lines. text must be unstyled — embedded SGR escapes are
// skipped by the width measure but still emitted, so style AFTER wrapping. width<=0
// returns the input split only on its existing '\n'.
func Wrap(text string, width int) []string {
	var lines []string
	// Split on the caller's hard breaks first, then word-wrap each segment.
	for _, seg := range strings.Split(text, "\n") {
		if width <= 0 {
			lines = append(lines, seg)
			continue
		}
		line := ""
		for _, word := range strings.Split(seg, " ") {
			// Start a new line when appending this word would overflow (but never
			// break a word that is itself wider than the column).
			if line != "" && VisibleWidth(line)+1+VisibleWidth(word) > width {
				lines = append(lines, line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		lines = append(lines, line)
	}
	return lines
}

// PageBG returns the SGR that selects the form's page background — a deep indigo
// from the selection-highlight family — so the whole frame sits on its own panel
// instead of the host terminal's theme. Empty string when styling is off or
// truecolor is unavailable.
func PageBG() string {
	if !StylingEnabled() || !TruecolorEnabled() {
		return ""
	}
	return "\033[48;2;20;12;40m" // deep indigo
}

// OnPage lays a whole multi-line frame onto the page background: every line is
// filled with PageBG() edge-to-edge across width columns, and the bg is re-armed
// after each interior SGR reset so styled spans don't punch holes back to the
// terminal's default. Returns frame unchanged when styling is off or width is 0.
// Pass the full terminal width.
func OnPage(frame string, width int) string {
	bg := PageBG()
	if bg == "" || width <= 0 {
		return frame
	}
	const reset = "\033[0m"
	var out strings.Builder
	lines := strings.Split(frame, "\n")
	for idx, line := range lines {
		// Re-arm the bg after every interior reset so a styled span's trailing
		// reset doesn't drop the rest of the line back to the terminal bg.
		var painted strings.Builder
		painted.WriteString(bg)
		p := 0
		for {
			r := strings.Index(line[p:], reset)
			if r < 0 {
				break
			}
			painted.WriteString(line[p : p+r+len(reset)])
			painted.WriteString(bg)
			p += r + len(reset)
		}
		painted.WriteString(line[p:])

		// Pad to ONE COLUMN SHY of full width under the bg, then reset once at
		// the line end. Writing into the terminal's LAST cell arms the
		// pending-wrap flag (DECAWM); the next line then wraps, inserting a blank
		// row between every line. Leaving the final column empty keeps the indigo
		// fill edge-to-edge visually while never tripping autowrap.
		fill := width - 1
		w := VisibleWidth(line)
		if w < fill {
			painted.WriteString(strings.Repeat(" ", fill-w))
		}
		painted.WriteString(reset)

		out.WriteString(painted.String())
		if idx < len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}

// --- Terminal control (best-effort, no-op when styling off) -----------------

// writeRaw emits straight to stdout. A failed write to the console has no
// recovery path (stdout IS the UI), so the error is intentionally discarded —
// the AGENTS "genuinely fire-and-forget" exemption.
func writeRaw(s string) {
	_, _ = fmt.Fprint(os.Stdout, s)
}

// SetTerminalBG sets the emulator's default background to the deep indigo via
// OSC 11. Unlike PageBG() (an SGR that only paints what we draw), this changes
// the emulator's default bg, so plain text printed afterwards — including a child
// process's output — sits on the deep-indigo page too. Best-effort; terminals
// that ignore OSC 11 keep their own bg. Pair every SetTerminalBG with a
// ResetTerminalBG. No-op when styling is off or truecolor is unavailable.
func SetTerminalBG() {
	if !StylingEnabled() || !TruecolorEnabled() {
		return
	}
	writeRaw("\033]11;#140c28\007") // #RRGGBB, BEL-terminated
}

// ResetTerminalBG resets the emulator's default background (OSC 111).
func ResetTerminalBG() {
	if !StylingEnabled() || !TruecolorEnabled() {
		return
	}
	writeRaw("\033]111\007")
}

// ResizeWindow asks the terminal to resize its window to rows x cols character
// cells via CSI "8 t" (DECSLPP-style window manipulation). Modern conhost and
// Windows Terminal honour it. Best-effort; terminals that ignore it keep their
// size. No-op when styling is off.
func ResizeWindow(rows, cols int) {
	if !StylingEnabled() || rows <= 0 || cols <= 0 {
		return
	}
	writeRaw(fmt.Sprintf("\033[8;%d;%dt", rows, cols))
}

// --- Selection / caret ------------------------------------------------------

// NeonBar is a "selected" highlight: text painted in the synthwave sunset ramp
// (magenta→coral→gold, per character) over a rich purple background, so the
// focused row/button glows like the sun against the cyan grid. Truecolor uses
// the per-char gradient; the 16-colour fallback uses reverse-video. Returns text
// unchanged when styling is off. text must be PLAIN (no nested SGR). width is the
// shared ramp span and start the bar's starting column, so the sunset gradient
// changes at the SAME rate as the title/field gradients (0 width → ramp over the
// bar's own length).
func NeonBar(text string, width, start int) string {
	if !StylingEnabled() {
		return text
	}
	if TruecolorEnabled() {
		const bg = "48;2;48;26;74" // muted purple
		rs := []rune(text)
		span := width
		if span <= 0 {
			span = len(rs)
		}
		var b strings.Builder
		col := start
		for _, run := range rs {
			t := 0.0
			if span > 1 {
				t = float64(col) / float64(span-1)
			}
			fg := sunsetRGB(t)
			fmt.Fprintf(&b, "\033[1;38;2;%d;%d;%d;%sm%s\033[0m",
				fg.r, fg.g, fg.b, bg, string(run))
			col++
		}
		return b.String()
	}
	return "\033[7m" + text + "\033[0m" // 16-colour: reverse-video
}

// Caret is a single-cell text caret for inline editing: ch (the character under
// the cursor, or a space at end-of-line) painted as a near-white glyph on a
// muted purple cell — calmer than NeonBar's sunset. Falls back to reverse-video
// in 16-colour and "[ch]" when styling is off.
func Caret(ch string) string {
	if !StylingEnabled() {
		return "[" + ch + "]"
	}
	if TruecolorEnabled() {
		return "\033[1;38;2;235;230;245;48;2;120;96;170m" + ch + "\033[0m"
	}
	return "\033[7m" + ch + "\033[0m"
}

// --- Banner -----------------------------------------------------------------

// gradientLine paints one art line with a left→right sunset gradient. width is
// the ramp's reference width so a stacked block shares one ramp. Returns the raw
// text (no colour) when styling is off, so it still reads in a pipe/log.
func gradientLine(line string, width int) string {
	if !StylingEnabled() {
		return line
	}
	var b strings.Builder
	rs := []rune(line)
	for i, run := range rs {
		t := 0.0
		if width > 1 {
			t = float64(i) / float64(width-1)
		}
		c := sunsetRGB(t)
		b.WriteString(RGB(c.r, c.g, c.b, string(run)))
	}
	return b.String()
}

func artWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		if n := len([]rune(l)); n > w {
			w = n
		}
	}
	return w
}

// Banner renders multi-line ASCII-art (the app wordmark, e.g. a figlet) with a
// left→right synthwave sunset gradient (truecolor) or per-char fallback
// (16-colour), centered within cols columns, followed by a centered tag line (e.g.
// "[ mass | 0.1.0 ]"). Returns a ready-to-print block. Plain ASCII art (no
// colour) when styling is off, so it still reads in a pipe/log.
//
// Unlike the worker's term.cpp (which hard-codes the MASS wordmark + "llama.cpp"
// tag), the art and tag are parameters so each app supplies its own identity.
func Banner(art []string, tag string, cols int) string {
	// cols is the width to center within (0 → Center is a no-op, left-aligned). The
	// caller passes the width its surrounding content centers on, so the wordmark
	// shares the same center axis as the rest of the frame rather than the live
	// terminal width (which is wider than a snug content box, shifting the banner).
	w := artWidth(art)

	var b strings.Builder
	if StylingEnabled() {
		b.WriteString("\n")
	}
	for _, line := range art {
		b.WriteString(Center(gradientLine(line, w), cols))
		b.WriteString("\n")
	}
	b.WriteString(Center(gradientLine(tag, len([]rune(tag))), cols))
	b.WriteString("\n\n") // breathing room before the first section
	return b.String()
}

// --- Status marks + progress ------------------------------------------------

// OKMark, FailMark, NoteMark are status glyphs for step results. Each is the
// glyph + a trailing gap, coloured when styling is on; on a non-styling stream
// they degrade to ASCII. Compose in front of a step label.
//
// The glyphs are "ambiguous width": Windows Terminal renders them two cells
// wide, so a single trailing space gets visually swallowed. Two trailing spaces
// guarantee a visible gap there and still read fine in 1-cell terminals.
func OKMark() string {
	if !StylingEnabled() {
		return "ok "
	}
	return Muted("✔") + "  " // muted lavender (the hint tone), not green, to stay on-palette
}

func FailMark() string {
	if !StylingEnabled() {
		return "x "
	}
	return Red("✖") + "  "
}

func NoteMark() string {
	if !StylingEnabled() {
		return "- "
	}
	return Accent("•") + "  "
}

// ProgressBar renders a determinate progress bar of width cells for done/total,
// e.g. "[██████░░] 6/8". Returned without a newline so a caller can rewrite it in
// place with a leading '\r'. Uses block glyphs when styling is on, '#'/'-'
// otherwise. total<=0 renders as an empty bar rather than dividing by zero.
func ProgressBar(done, total, width int) string {
	if total <= 0 {
		total = 1
	}
	if done > total {
		done = total
	}
	if done < 0 {
		done = 0
	}
	filled := (width * done) / total
	empty := width - filled
	count := fmt.Sprintf(" %d/%d", done, total)

	if !StylingEnabled() {
		return "[" + strings.Repeat("#", filled) + strings.Repeat("-", empty) + "]" + count
	}
	return "[" + Cyan(strings.Repeat("█", filled)) + Dim(strings.Repeat("░", empty)) + "]" + count
}
