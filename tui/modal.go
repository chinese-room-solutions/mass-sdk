package tui

import (
	"errors"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/term"
)

// ErrDeclined means the environment can't host an interactive screen (no tty /
// raw mode refused). Confirm, BackOrExit, and ErrorScreen return it so the
// caller falls back to a plain linear prompt — it is NOT a user cancel (a user
// cancel resolves to a button choice, e.g. Confirm's "No").
var ErrDeclined = errors.New("tui: terminal cannot host the interactive screen")

// The wrap+margin modal-message model (ModalLine's Prose/Error/Styled kinds,
// splitSentences, renderModalLine) mirrors the worker's C++ modal in
// mass-worker-llama-cpp (src/form.cpp). The two are independent copies, so a
// change to the message-rendering rules here should be mirrored there, and vice
// versa, to keep both installers' modals identical.

// modalMargin is the blank cells kept each side of a wrapped modal message line,
// so a long message (a sudo confirm, an error) never runs edge-to-edge.
const modalMargin = 5

// ModalLineKind is how a modal message row is rendered.
type ModalLineKind uint8

const (
	// ModalProse is plain text (a confirm question): the modal splits it into
	// sentences, word-wraps to the margin, and paints it with the cool grid ramp.
	ModalProse ModalLineKind = iota
	// ModalError is a plain abort message: split + wrapped like prose, but painted
	// red (✖ + accent) so it reads as a failure.
	ModalError
	// ModalStyled is a pre-composed row (a summary mixing marks / dim labels) shown
	// verbatim — only centered, never wrapped or recoloured.
	ModalStyled
)

// ModalLine is one message row above a modal's buttons.
type ModalLine struct {
	Text string
	Kind ModalLineKind
}

// ModalSpec describes a two-button modal on the synthwave panel.
type ModalSpec struct {
	BannerArt []string
	Tag       string

	// Lines are the message rows shown above the buttons. Prose/Error lines are
	// wrapped to the margin and coloured per kind; Styled lines show verbatim.
	Lines []ModalLine

	// Buttons are the two labels (left, right). Selected is the index focused on
	// entry, so a plain Enter takes that choice.
	Buttons  [2]string
	Selected int

	// Footer is the muted hint line below the buttons.
	Footer string

	// Shortcut maps a lowercased key byte to a button index (e.g. 'y'->0), or -1
	// for no match. Optional.
	Shortcut func(byte) int

	// OnCancel is the index returned for Esc/Ctrl-C/EOF.
	OnCancel int
}

// splitSentences puts each sentence on its own line: a ". " (or "? " / "! ")
// boundary becomes a '\n', which Wrap treats as a hard break. The terminator stays
// on the sentence it closes.
func splitSentences(prose string) string {
	var b strings.Builder
	for i := 0; i < len(prose); i++ {
		b.WriteByte(prose[i])
		boundary := (prose[i] == '.' || prose[i] == '?' || prose[i] == '!') &&
			i+1 < len(prose) && prose[i+1] == ' '
		if boundary {
			b.WriteByte('\n')
			i++ // consume the space that followed the terminator
		}
	}
	return b.String()
}

// renderModalLine wraps + colours one message line into rendered rows (each still
// to be centered by the caller). Prose/Error split into sentences and wrap to
// width; Styled passes through unchanged. Error prefixes the first row with the ✖
// mark and paints every row in the accent colour.
func renderModalLine(line ModalLine, width int) []string {
	if line.Kind == ModalStyled {
		return []string{line.Text}
	}
	pieces := term.Wrap(splitSentences(line.Text), width)
	rows := make([]string, 0, len(pieces))
	for i, p := range pieces {
		if line.Kind == ModalError {
			prefix := "  "
			if i == 0 {
				prefix = term.FailMark()
			}
			rows = append(rows, prefix+term.Accent(p))
		} else {
			rows = append(rows, term.CoolGradient(p, len(p), 0))
		}
	}
	return rows
}

// composeModal builds a modal's whole frame (banner → message lines → buttons →
// footer) centered within cols, without the page-bg fill or vertical centering.
// Pure, so it is unit/golden-testable; runModal wraps it with the screen fill.
func composeModal(spec ModalSpec, selected, cols int) string {
	var out strings.Builder
	out.WriteString(term.Banner(spec.BannerArt, spec.Tag, cols))
	out.WriteString("\n")
	// Each line wraps to the margin + colours per its kind, then centers on the
	// window axis, so no message runs edge-to-edge.
	for _, line := range spec.Lines {
		for _, row := range renderModalLine(line, cols-2*modalMargin) {
			out.WriteString(term.Center(row, cols) + "\n")
		}
	}
	out.WriteString("\n")

	// Focused button = self-contained sunset bar, the other = cool grid
	// gradient — matching the form's action row.
	var row strings.Builder
	for i, b := range spec.Buttons {
		btn := "[ " + b + " ]"
		if i == selected {
			row.WriteString(term.NeonBar(btn, len(btn), 0))
		} else {
			row.WriteString(term.CoolGradient(btn, len(btn), 0))
		}
		if i == 0 {
			row.WriteString("   ")
		}
	}
	out.WriteString(term.Center(row.String(), cols) + "\n\n")
	out.WriteString(term.Center(term.Muted(spec.Footer), cols) + "\n")
	return out.String()
}

// runModal renders + drives a two-button modal, returning the chosen index.
// Assumes a RawMode + altScreen are already active.
func runModal(raw *RawMode, spec ModalSpec) int {
	// Drop any keystroke queued before this screen (e.g. the Enter that submitted
	// a sudo password) so it can't auto-action a button before the operator sees
	// the modal.
	raw.DiscardPending()
	selected := spec.Selected

	render := func() {
		// Compose within the content band, then shift the composed block to the
		// band's column, so the modal sits on the window's axis like the form's
		// frame does. OnPage gets the FULL window width regardless: the page
		// background has to paint every row edge-to-edge, not just the band.
		cols := term.TerminalWidth()
		out := term.IndentBlock(composeModal(spec, selected, min(cols, term.ContentWidth)),
			term.ContentOrigin(cols))

		var full strings.Builder
		if term.StylingEnabled() {
			full.WriteString(term.PageBG() + "\033[2J\033[H")
		}
		full.WriteString(term.OnPage(verticallyCenter(out, termSizeFn().rows), cols))
		writeRaw(full.String())
	}

	for {
		render()
		k, err := raw.ReadKey()
		if err != nil {
			return spec.OnCancel
		}
		switch k.Type {
		case KeyLeft, KeyRight, KeyTab:
			selected = (selected + 1) % 2
		case KeyEnter:
			return selected
		case KeyEsc, KeyCtrlC, KeyEof:
			return spec.OnCancel
		case KeyChar:
			c := lowerByte(k.Byte)
			if c == 'h' || c == 'l' {
				selected = (selected + 1) % 2
			} else if spec.Shortcut != nil {
				if hit := spec.Shortcut(c); hit >= 0 {
					return hit
				}
			}
		}
	}
}

// Confirm shows a themed yes/no confirmation on the synthwave panel with
// selectable [ Yes ] / [ No ] buttons (move with ←/→ or Tab, confirm with Enter,
// or press y/n directly). lines are PROSE — plain text the modal splits into
// sentences, word-wraps to the margin, and paints with the cool grid ramp — so
// pass them unstyled. defaultYes sets which button is focused on entry. Returns
// the choice, or ErrDeclined when raw terminal mode is unavailable — the caller
// then falls back to a plain text prompt. Esc / Ctrl-C / EOF → "no".
func Confirm(bannerArt []string, tag string, lines []string, defaultYes bool) (bool, error) {
	raw, err := enterRawModeFn()
	if err != nil {
		return false, ErrDeclined
	}
	defer raw.Close()
	alt := enterAltScreen(0, 0)
	defer alt.leave()

	selected := 1
	if defaultYes {
		selected = 0
	}
	chosen := runModal(raw, ModalSpec{
		BannerArt: bannerArt,
		Tag:       tag,
		Lines:     proseLines(lines),
		Buttons:   [2]string{"Yes", "No"},
		Selected:  selected,
		Footer:    "←/→ or Tab move · Enter confirm · y / n · Esc cancels",
		Shortcut: func(c byte) int {
			if c == 'y' {
				return 0
			}
			if c == 'n' {
				return 1
			}
			return -1
		},
		OnCancel: 1,
	})
	return chosen == 0, nil
}

// BackOrExit shows a themed end-of-action screen: lines (e.g. an install result)
// on the synthwave panel with selectable [ Back ] / [ Exit ] buttons (Back
// focused by default). lines are STYLED — pre-composed by the caller (a summary
// mixing marks + dim labels) — so they show verbatim, padded to one width and
// centered as a block; SummaryRows builds them already positioned on the form's
// grid, which this leaves alone. Returns (true, nil) for Back, (false, nil) for
// Exit, or ErrDeclined when raw mode is unavailable — the caller then falls back to
// a plain "press Enter" acknowledgement.
func BackOrExit(bannerArt []string, tag string, lines []string) (back bool, err error) {
	lines = alignBlock(lines)
	styled := make([]ModalLine, len(lines))
	for i, l := range lines {
		styled[i] = ModalLine{Text: l, Kind: ModalStyled}
	}
	return backOrExit(bannerArt, tag, styled)
}

// alignBlock right-pads every line to the widest one's visible width. The modal
// centers each line independently, so rows of different width would get different
// left margins and a summary's label column would stagger; padded, the block
// centers as a unit and stays internally left-aligned.
func alignBlock(lines []string) []string {
	w := 0
	for _, l := range lines {
		w = max(w, term.VisibleWidth(l))
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = padTo(l, w)
	}
	return out
}

// ErrorScreen shows the [ Back ] / [ Exit ] screen for an ERROR: message is plain
// text, word-wrapped to the margin and painted red (✖ + accent) so a long abort
// message reads as a failure and never runs edge-to-edge. Same return contract as
// BackOrExit.
func ErrorScreen(bannerArt []string, tag, message string) (back bool, err error) {
	return backOrExit(bannerArt, tag, []ModalLine{{Text: message, Kind: ModalError}})
}

// backOrExit is the shared [ Back ] / [ Exit ] modal body, given the already-built
// message lines.
func backOrExit(bannerArt []string, tag string, lines []ModalLine) (back bool, err error) {
	raw, rerr := enterRawModeFn()
	if rerr != nil {
		return false, ErrDeclined
	}
	defer raw.Close()
	alt := enterAltScreen(0, 0)
	defer alt.leave()

	chosen := runModal(raw, ModalSpec{
		BannerArt: bannerArt,
		Tag:       tag,
		Lines:     lines,
		Buttons:   [2]string{"Back", "Exit"},
		Selected:  0,
		Footer:    "←/→ or Tab move · Enter select · b / e · Esc exits",
		Shortcut: func(c byte) int {
			if c == 'b' {
				return 0
			}
			if c == 'e' {
				return 1
			}
			return -1
		},
		OnCancel: 1,
	})
	return chosen == 0, nil
}

// proseLines wraps each plain string as a ModalProse line.
func proseLines(lines []string) []ModalLine {
	out := make([]ModalLine, len(lines))
	for i, l := range lines {
		out[i] = ModalLine{Text: l, Kind: ModalProse}
	}
	return out
}

// lowerByte lowercases an ASCII byte (avoids unicode.ToLower for a single byte).
func lowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
