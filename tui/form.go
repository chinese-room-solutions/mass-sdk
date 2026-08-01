package tui

import (
	"errors"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/term"
)

// The form's field grid is the shared three-column MenuLayout — label | gutter |
// value — anchored so the gutter's midpoint sits on the centering axis (see
// menu.go). The label column is fixed, so the columns never reflow between forms
// with different labels; a label longer than it clips with "…" rather than shoving
// the value column. These are the DEFAULTS (sized for typical installer labels,
// e.g. "Listen address (host:port)" plus the marker); a form whose labels need
// more room overrides individual columns via FormSpec.Layout.
var formMenuLayout = MenuLayout{
	Marker:      2,           // "> " / "  ", drawn inside the label column
	LabelCol:    28,          // fits typical installer labels + marker
	Gap:         term.Gutter, // the gutter column: dead-centred on the axis
	ValueCol:    30,          // value column width (values clip to it with "…")
	MinValueCol: 16,          // floor for the value column on a too-narrow axis
}

// Layout constants for the form's frame.
const (
	formMarker = 2 // must match formMenuLayout.Marker (used by tests/edit row)

	// Minimum terminal size below which RunForm declines (caller falls back to a
	// linear flow rather than render a clipped screen).
	formMinRows = 22
	formMinCols = 50

	// The empty border, in character cells, the form keeps on each side. The
	// content is measured to its own box (widest line × content rows), then these
	// margins are wrapped around it and the window is snapped to the total — so the
	// same breathing room shows on every platform. Left/right are symmetric; the top
	// runs one short of the bottom because the banner's first row is top-light
	// (glyphs sit at the cell bottom), which reads as ~half a row of extra space.
	formMarginTop    = 2
	formMarginBottom = 3
	formMarginLeft   = 5
	formMarginRight  = 5
)

// FormSpec describes a form to run. The engine is generic over Fields and
// Actions — the caller (app) builds the field list and supplies the action
// button labels; nothing app-specific lives in the package.
type FormSpec struct {
	// BannerArt is the multi-line ASCII wordmark; Tag is the bracketed subtitle
	// (e.g. "[ mass | 0.1.0 ]"). Rendered via term.Banner.
	BannerArt []string
	Tag       string

	// Hint is the line shown under the banner (e.g. usage). Optional.
	Hint string

	Fields  []Field
	Actions []string // button labels, left→right (e.g. "Install", "Remove", "Exit")

	// Layout overrides the field grid's column geometry. Zero fields keep the
	// package defaults, so a form can override just what it needs — e.g. widen
	// LabelCol when its longest label would clip under the default.
	Layout MenuLayout

	// Status is an optional initial status/hint line (e.g. a warning).
	Status string

	// OnFieldEdited, when set, is called after a field's inline edit commits with
	// a changed value, receiving the field index and the current fields. It may
	// return a replacement field slice (e.g. re-seed downstream fields when a
	// "data directory" changes) or nil to keep the current fields. Pure-ish: it
	// must not touch the terminal. This is the one extensibility seam the worker
	// used (data-dir reload); keeping it generic avoids baking that into tui.
	OnFieldEdited func(index int, fields []Field) []Field

	// ResizeOnEnter snaps a full-window terminal to the form grid on entry
	// (Windows-style). Leave false for environments that size at launch.
	ResizeOnEnter bool
}

// FormResult is what RunForm returns. ActionIndex is the chosen button (valid
// only when neither Cancelled nor Declined); Fields is the edited field list.
// The two failure signals are distinct so callers can react differently:
//
//   - Declined: the environment can't host the TUI (terminal too small, no
//     raw mode) — fall back to a linear prompt flow.
//   - Cancelled: the operator aborted (Ctrl-C/EOF) — exit the flow.
type FormResult struct {
	ActionIndex int
	Fields      []Field
	Cancelled   bool
	Declined    bool
}

// formState is what the loop mutates.
type formState struct {
	spec         FormSpec
	fields       []Field
	cursor       int // 0..len(fields): == len → action row
	actionCursor int // which button on the action row
	status       string
	editing      bool
	editCursor   int // caret byte offset within the edited value

	// contentW is the width (cols) the body is composed and internally centered
	// against — the form's natural content width, measured once on entry and fixed
	// so the side margins stay stable even if OnFieldEdited re-seeds fields.
	contentW int

	// gridRows / gridCols are the window size the form snapped to (ResizeOnEnter),
	// i.e. the content box plus its margins; 0 means no resize (the layout then
	// anchors to the live window, with the same top/left margins).
	// renderForm lays out against these targets rather than a live size query:
	// GetConsoleScreenBufferInfo can still report the pre-resize window for a frame
	// or two after an async CSI 8t, which would mis-place the content and leave a
	// scrollbar.
	gridRows int
	gridCols int
}

func (st *formState) onActionRow() bool { return st.cursor == len(st.fields) }

// menuLayout resolves the form's field-grid geometry: the package defaults with
// any non-zero FormSpec.Layout fields applied on top.
func (spec FormSpec) menuLayout() MenuLayout {
	l := formMenuLayout
	if spec.Layout.Marker > 0 {
		l.Marker = spec.Layout.Marker
	}
	if spec.Layout.LabelCol > 0 {
		l.LabelCol = spec.Layout.LabelCol
	}
	if spec.Layout.Gap > 0 {
		l.Gap = spec.Layout.Gap
	}
	if spec.Layout.ValueCol > 0 {
		l.ValueCol = spec.Layout.ValueCol
	}
	if spec.Layout.MinValueCol > 0 {
		l.MinValueCol = spec.Layout.MinValueCol
	}
	return l
}

// RunForm runs the interactive single-screen form. It returns a FormResult on a
// clean finish (including a chosen Exit-style action); Declined=true when the
// terminal is too small or raw mode is unavailable (the caller then falls back
// to a linear flow); Cancelled=true when the operator aborts with Ctrl-C/EOF
// (the caller exits). The terminal is restored on every exit path (defer).
func RunForm(spec FormSpec) (FormResult, error) {
	st := &formState{spec: spec, fields: append([]Field(nil), spec.Fields...), status: spec.Status}
	normalizeChoices(st.fields)

	// Measure the form's natural content box once, up front: the width the body
	// wants (its widest line) and the rows it occupies. Fixed for the form's
	// lifetime — the width floors renderForm's live centering (never below the box,
	// or a wide line would spill) and, plus the margins, sizes the snapped window on
	// Windows; the height sizes the window and gates the decline check.
	st.contentW = formContentWidth(st)
	contentRows := formContentHeight(st)

	// Decline only when the terminal genuinely can't host the form's CONTENT (the
	// margin doesn't count — it just collapses in a tight window). The row floor is the
	// content height, capped at formMinRows: a bundle launches its terminal already
	// sized to the form (~content height, below formMinRows), so gating on the fixed
	// minimum would wrongly drop it to the linear fallback and flash closed. This
	// holds whether or not the form resizes the window itself.
	//
	// The column floor is the widest the layout CAN'T shrink past — the field grid's
	// minimum block (LabelCol + Gap + MinValueCol) plus the two side margins — not just
	// the absolute formMinCols. Below it the grid would overrun its own right edge
	// however narrow we compose, so the linear fallback is the honest outcome; at or
	// above it the wide single-line elements word-wrap and every row fits.
	l := spec.menuLayout()
	minCols := max(formMinCols, l.LabelCol+l.Gap+l.MinValueCol+formMarginLeft+formMarginRight)
	minRows := min(formMinRows, contentRows)
	if sz := termSizeFn(); sz.rows < minRows || sz.cols < minCols {
		return FormResult{Declined: true}, nil
	}

	raw, err := enterRawModeFn()
	if err != nil {
		// Not a tty / raw mode refused → linear fallback (not an error condition).
		return FormResult{Declined: true}, nil
	}
	defer raw.Close()

	rr, rc := 0, 0
	if spec.ResizeOnEnter {
		// Snap the window to the content box plus its four margins. Sizing to the
		// content — not the window's current full-window size — is what keeps the
		// same band of breathing room on every platform (and keeps a short form from
		// leaving a tall empty gutter below it, with the scrollbar a taller-than-
		// window buffer trips). A host that pins its size (e.g. konsole) just keeps
		// the form centered with whatever slack remains. This window IS the form's:
		// the result/confirm modals reuse it (they don't resize), so it must fit the
		// form, the largest view.
		rr = contentRows + formMarginTop + formMarginBottom
		rc = st.contentW + formMarginLeft + formMarginRight
		st.gridRows, st.gridCols = rr, rc
	}
	alt := enterAltScreen(rr, rc)
	defer alt.leave()

	for {
		renderForm(st)

		k, err := raw.ReadKey()
		if err != nil {
			return FormResult{Cancelled: true}, err
		}
		if k.Type == KeyCtrlC || k.Type == KeyEof {
			return FormResult{Cancelled: true}, nil
		}

		if !st.onActionRow() {
			if done, res := handleFieldKey(raw, st, k); done {
				return res, nil
			}
		} else {
			if done, res := handleActionKey(st, k); done {
				return res, nil
			}
		}
	}
}

// handleFieldKey processes a key while the cursor is on a field row. Returns
// done=true (with a result) only when an inline edit signals a hard cancel.
func handleFieldKey(raw *RawMode, st *formState, k Key) (bool, FormResult) {
	f := &st.fields[st.cursor]
	switch k.Type {
	case KeyUp:
		if st.cursor > 0 {
			st.cursor--
		} else {
			// Wrap from the first field up onto the action row (first button).
			st.cursor = len(st.fields)
			st.actionCursor = 0
		}
	case KeyDown:
		st.cursor++ // may step onto the action row (== len)
	case KeyTab:
		st.cursor++
		st.actionCursor = 0
	case KeyChar:
		switch {
		case k.Byte == 'k' && st.cursor > 0:
			st.cursor--
		case k.Byte == 'j':
			st.cursor++
		case k.Byte == 'e' && f.Kind != FieldChoice:
			// 'e' always edits inline — the way to text-edit a path field whose
			// Enter opens the picker, and a synonym for Enter on other fields.
			if !editField(raw, st) {
				return true, FormResult{Cancelled: true}
			}
		}
	case KeyLeft:
		if f.Kind == FieldChoice {
			cycleAndNotify(st, f, -1)
		}
	case KeyRight:
		if f.Kind == FieldChoice {
			cycleAndNotify(st, f, +1)
		}
	case KeyEnter:
		switch f.Kind {
		case FieldChoice:
			// Choices cycle with ←/→; Enter on the field row does nothing.
		case FieldPath:
			// Enter browses with the native picker. Only when there's no picker on
			// this platform does it fall back to inline editing; a pick or a cancel
			// stays out of the editor (the user pressed Enter to browse, not 'e').
			if browsePath(st, f) {
				if !editField(raw, st) {
					return true, FormResult{Cancelled: true}
				}
			}
		default:
			// A non-path editable field → edit inline.
			if !editField(raw, st) {
				return true, FormResult{Cancelled: true}
			}
		}
	}
	return false, FormResult{}
}

// editField inline-edits the field under the cursor and fires OnFieldEdited if
// the value changed. Returns false on a hard cancel (Ctrl-C / EOF) so the caller
// aborts the whole form. Shared by Enter and the 'e' shortcut.
func editField(raw *RawMode, st *formState) bool {
	idx := st.cursor
	f := &st.fields[idx]
	before := f.Value
	if !editInline(raw, st, f) {
		return false
	}
	notifyEdited(st, idx, before)
	return true
}

// pickFolderFn is the picker the form calls; a package var so tests can stub it.
var pickFolderFn = PickFolder

// browsePath opens the native folder picker for a path field. It returns
// fallbackEdit=true ONLY when there's no picker on this platform, so the caller
// then edits inline; a user cancel returns false (leave the field as-is — do NOT
// drop into the editor, since the user pressed Enter to browse, not 'e' to edit).
// On a chosen path it updates the value (re-validating, firing OnFieldEdited on a
// change). A picker error is shown in the status line and does not fall back.
func browsePath(st *formState, f *Field) (fallbackEdit bool) {
	idx := st.cursor
	before := f.Value
	path, ok, err := pickFolderFn("Choose " + strings.ToLower(f.Label))
	switch {
	case errors.Is(err, ErrNoPicker):
		return true // no native dialog → edit inline
	case err != nil:
		st.status = err.Error()
		return false
	case !ok:
		return false // user cancelled → leave the value, don't open the editor
	}
	f.Value = path
	if msg := ValidateField(*f); msg != "" {
		st.status = msg
		f.Value = before
		return false
	}
	st.status = ""
	notifyEdited(st, idx, before)
	return false
}

// cycleAndNotify advances a choice field and, if the value changed, fires the
// OnFieldEdited hook — so cycling a choice (e.g. an install scope) re-seeds
// dependent fields the same way an inline text edit does.
func cycleAndNotify(st *formState, f *Field, delta int) {
	idx := st.cursor
	before := f.Value
	CycleChoice(f, delta)
	notifyEdited(st, idx, before)
}

// notifyEdited calls OnFieldEdited when field idx's value changed from before,
// swapping in any replacement field slice it returns (clamping the cursor).
func notifyEdited(st *formState, idx int, before string) {
	if st.fields[idx].Value == before || st.spec.OnFieldEdited == nil {
		return
	}
	if next := st.spec.OnFieldEdited(idx, st.fields); next != nil {
		normalizeChoices(next)
		st.fields = next
		st.cursor = min(st.cursor, len(st.fields))
	}
}

// handleActionKey processes a key while the cursor is on the action row. Returns
// done=true (with a result) when an action commits.
func handleActionKey(st *formState, k Key) (bool, FormResult) {
	n := len(st.spec.Actions)
	lastField := len(st.fields) - 1
	switch k.Type {
	case KeyLeft:
		st.actionCursor = (st.actionCursor + n - 1) % n
	case KeyRight:
		st.actionCursor = (st.actionCursor + 1) % n
	case KeyTab:
		if st.actionCursor+1 < n {
			st.actionCursor++
		} else {
			st.cursor = 0
			st.actionCursor = 0
		}
	case KeyUp:
		st.cursor = lastField
	case KeyDown:
		st.cursor = 0
	case KeyChar:
		switch k.Byte {
		case 'h':
			st.actionCursor = (st.actionCursor + n - 1) % n
		case 'l':
			st.actionCursor = (st.actionCursor + 1) % n
		case 'k':
			st.cursor = lastField
		case 'j':
			st.cursor = 0
		}
	case KeyEnter:
		if bad, ok := firstInvalid(st.fields); ok {
			st.cursor = bad
			st.status = ValidateField(st.fields[bad])
			return false, FormResult{}
		}
		return true, FormResult{
			ActionIndex: st.actionCursor,
			Fields:      st.fields,
		}
	}
	return false, FormResult{}
}

// editInline edits a text/secret/path/int field on its own row: renderForm draws
// the live value + a caret while st.editing is set. Reads raw keys until Enter
// commits (subject to validation) or Esc reverts. Returns false on a hard cancel
// (Ctrl-C / EOF) so the caller aborts the whole form.
func editInline(raw *RawMode, st *formState, f *Field) bool {
	before := f.Value
	st.editing = true
	st.editCursor = len(f.Value) // caret starts at end
	defer func() { st.editing = false }()

	for {
		renderForm(st)

		k, err := raw.ReadKey()
		if err != nil {
			return false
		}
		clamp := func() {
			if st.editCursor > len(f.Value) {
				st.editCursor = len(f.Value)
			}
		}

		switch k.Type {
		case KeyCtrlC, KeyEof:
			return false
		case KeyEsc:
			f.Value = before // revert
			return true
		case KeyEnter:
			if msg := ValidateField(*f); msg != "" {
				st.status = msg
				f.Value = before // keep the prior valid value on reject
				clamp()
				continue
			}
			st.status = ""
			return true
		case KeyLeft:
			if st.editCursor > 0 {
				st.editCursor--
			}
		case KeyRight:
			if st.editCursor < len(f.Value) {
				st.editCursor++
			}
		case KeyCtrlLeft:
			st.editCursor = wordLeft(f.Value, st.editCursor)
		case KeyCtrlRight:
			st.editCursor = wordRight(f.Value, st.editCursor)
		case KeyHome:
			st.editCursor = 0
		case KeyEnd:
			st.editCursor = len(f.Value)
		case KeyBackspace:
			if st.editCursor > 0 {
				f.Value = f.Value[:st.editCursor-1] + f.Value[st.editCursor:]
				st.editCursor--
			}
		case KeyChar:
			f.Value = f.Value[:st.editCursor] + string(k.Byte) + f.Value[st.editCursor:]
			st.editCursor++
		}
	}
}

// renderForm composes the whole frame into one string and emits it with a single
// write, so the redraw is flicker-free. Cursor home + clear-to-page-bg each frame.
//
// Layout is content-box + margin: the body is composed and internally centered
// against its own natural width (st.contentW), then wrapped with the form's blank
// border (the formMargin* family, one per side) so the form reads the same across
// platforms regardless of the host window size.
func renderForm(st *formState) {
	// The window the content box lives in. When we snapped it (ResizeOnEnter), trust
	// that target over a live query — an async CSI 8t can leave the live size
	// reporting the old window for a frame, which would mis-center and trip a
	// scrollbar. Otherwise use the live size (a launch-sized bundle terminal).
	winRows, winCols := st.gridRows, st.gridCols
	if winRows <= 0 {
		sz := termSizeFn()
		winRows, winCols = sz.rows, sz.cols
	}

	// Compose within the width the window can actually show — (winCols − the two side
	// margins) — and let frameWithMargin re-add the left margin, landing the centering
	// axis on winCols/2 for any width. A host can open WIDER than the grid we asked for
	// (konsole's --qwindowgeometry is unreliable); composing at the stale natural box
	// would then anchor everything formMarginLeft from the left with the surplus piled
	// on the right. It can also open NARROWER than the box — a host that ignores the
	// CSI 8t self-resize keeps its default (e.g. 80×24) — so we do NOT floor at
	// contentW: flooring there makes a too-narrow window compose past its own right
	// edge and overrun the margin. Below the box, composeBody degrades in place (the
	// field grid shrinks its value column, the hint/footer word-wrap) rather than
	// spilling. Guard only a sane minimum so the field gutter math stays positive.
	centerW := max(winCols, formMinCols) - formMarginLeft - formMarginRight
	body := composeBody(st, centerW)
	frame := frameWithMargin(body, winRows)

	var full strings.Builder
	if term.StylingEnabled() {
		full.WriteString(term.PageBG() + "\033[2J\033[H")
	}
	full.WriteString(term.OnPage(frame, winCols))
	writeRaw(full.String())
}

// frameWithMargin wraps the composed body (already internally centered to a
// contentW-wide box) in the form's margins (the formMargin* family, one per side),
// then fills the rest of the window so every row is painted (an unpainted bottom
// reads as scrollback and shows a scrollbar).
//
// The content box is anchored top-left at exactly (formMarginTop, formMarginLeft)
// on EVERY platform — the margins to the content are identical whether or not the
// window was snapped to the box. When the host won't shrink to the box (e.g. a
// Linux/macOS terminal that opened larger than requested), the leftover width and
// height become extra right/bottom fill, not centering — so the top and left
// margins a user sees never drift from the Windows-snapped case.
func frameWithMargin(body string, winRows int) string {
	// The banner/action/status rows carry their own leading/trailing blank lines
	// (a styled banner starts with "\n", the action+status rows end with a couple).
	// Trim them so the margin is measured against the FIRST and LAST rows that
	// actually have content — otherwise the visible top/bottom border would be the
	// margin plus those incidental blanks.
	lines := trimBlankEdges(strings.Split(body, "\n"))

	// Horizontal: indent every body line by exactly formMarginLeft. (OnPage pads
	// each line out to the window width afterwards, so a window wider than the box
	// just gets more painted right-fill — the visible left margin stays put.)
	indent := strings.Repeat(" ", formMarginLeft)
	var indented strings.Builder
	for i, line := range lines {
		if i > 0 {
			indented.WriteString("\n")
		}
		indented.WriteString(indent)
		indented.WriteString(line)
	}

	// Vertical: the box sits at formMarginTop when the window fits it snugly (the
	// Windows-snapped and correctly-sized-bundle case — no surplus). When the window
	// is TALLER than the box + its margins (the resize was ignored, or a host that
	// won't shrink), split the surplus so the box is vertically centered, slightly
	// top-biased (the 2/5 the modals use) — otherwise all the extra height floods the
	// bottom and the form looks bottom-heavy in an over-tall window. The top pad never
	// drops below formMarginTop, so the top margin a user sees never shrinks; the
	// bottom keeps at least formMarginBottom.
	total := len(lines) + formMarginTop + formMarginBottom
	topPad, botPad := formMarginTop, formMarginBottom
	if surplus := winRows - total; surplus > 0 {
		topPad += (surplus * 2) / 5
		botPad = winRows - len(lines) - topPad
	}
	var b strings.Builder
	b.WriteString(strings.Repeat("\n", topPad))
	b.WriteString(indented.String())
	b.WriteString(strings.Repeat("\n", botPad))
	return b.String()
}

// trimBlankEdges drops leading and trailing all-whitespace lines, so the margin
// wraps the first/last rows with actual content. Returns a single empty line for
// an all-blank input (never an empty slice).
func trimBlankEdges(lines []string) []string {
	lo, hi := 0, len(lines)-1
	for lo <= hi && strings.TrimSpace(lines[lo]) == "" {
		lo++
	}
	for hi >= lo && strings.TrimSpace(lines[hi]) == "" {
		hi--
	}
	if lo > hi {
		return []string{""}
	}
	return lines[lo : hi+1]
}

// composeBody builds the form's content (banner → fields → actions → status) as
// one string, without the page-bg fill or vertical centering. cols is the width
// the single-line elements center within and the field menu's gutter anchors on
// (0 → no centering, everything left-flush at its intrinsic width). Pure: callers
// use it both to render and to measure the form's natural box.
func composeBody(st *formState, cols int) string {
	layout := st.spec.menuLayout()
	geo := MenuGeometryFor(layout, cols)

	var out strings.Builder
	// Center the banner on the SAME width the fields/hint/footer center on (cols),
	// not the live terminal width — so the wordmark shares the content box's center
	// axis instead of drifting toward the (wider) window center.
	out.WriteString(term.Banner(st.spec.BannerArt, st.spec.Tag, cols))
	if st.spec.Hint != "" {
		out.WriteString(centerWrapped(st.spec.Hint, cols, term.Muted))
		out.WriteString("\n\n")
	}

	for i := range st.fields {
		out.WriteString(renderFieldRow(st, i, layout, geo))
		out.WriteString("\n")
	}

	out.WriteString(renderActionRow(st, cols))
	out.WriteString(renderStatusLine(st, cols))
	return out.String()
}

// formContentHeight is the rows the form's content occupies (banner → fields →
// actions → status), without any margin. The window is sized to this plus the
// top+bottom margins, and it's the real floor for hosting the form (a window that
// fits the content but not the margin still renders — the margin just collapses),
// so the decline check uses it too. Row count is independent of the width we
// compose at.
func formContentHeight(st *formState) int {
	body := composeBody(st, st.contentW)
	// Count the rows that actually carry content — the same trim frameWithMargin
	// applies — so the snapped window height matches the rendered frame and the
	// bottom margin lands at exactly formMarginBottom.
	return len(trimBlankEdges(strings.Split(body, "\n")))
}

// formContentWidth is the form's natural content width in columns: the widest
// visible line of the body, capped at the content band. It composes at cols=0 — where
// the per-line centering (term.Center, the field gutter indent) is a no-op, so
// each line is left-flush at its INTRINSIC width — and takes the max. Measuring
// the intrinsic width (not a centered line's offset position, which grows with the
// width centered within) is what makes the box edges land at the true margins: the
// body is then re-composed and centered within exactly this width, so the box sits
// formMarginLeft/Right from the window edge.
func formContentWidth(st *formState) int {
	body := composeBody(st, 0)
	w := 0
	for _, line := range strings.Split(body, "\n") {
		if vw := term.VisibleWidth(line); vw > w {
			w = vw
		}
	}
	return min(w, term.ContentWidth)
}

// renderFieldRow renders one field row via the shared menu component. The plain
// (unselected) and selected rows go straight through RenderMenuRow; the row being
// inline-edited is drawn here because it carries a live caret the menu doesn't
// model — it reuses MenuLabelCell so its columns stay aligned with the rest.
func renderFieldRow(st *formState, i int, layout MenuLayout, geo MenuGeometry) string {
	f := st.fields[i]
	current := !st.onActionRow() && i == st.cursor

	if current && st.editing {
		label := MenuLabelCell(f.Label, layout, true)
		val := renderEditValue(f, st.editCursor, geo.ValueWidth)
		return strings.Repeat(" ", geo.Indent) + term.Bold(label) + val
	}

	// Choices carry their bare value + the IsChoice flag so the menu adds the "< >"
	// brackets; other fields pass their display form (secrets masked). Double-
	// bracketing would show "< < User > >".
	row := MenuRow{Left: f.Label, IsChoice: f.Kind == FieldChoice}
	if f.Kind == FieldChoice {
		row.Right = f.Value
	} else {
		row.Right = displayValue(f)
	}
	if current {
		row.Style = MenuRowSelected
	}
	return RenderMenuRow(row, layout, geo)
}

// renderEditValue draws the value with a caret at editCursor, scrolling a window
// so the caret stays visible when the value is longer than valueWidth.
func renderEditValue(f Field, editCursor, valueWidth int) string {
	full := displayValue(f)
	cursor := min(editCursor, len(full))
	start := 0
	if len(full) > valueWidth && cursor > valueWidth-1 {
		start = cursor - (valueWidth - 1) // keep the caret in view
	}
	end := min(start+valueWidth, len(full))
	window := full[start:end]
	caretAt := cursor - start

	atEnd := caretAt >= len(window)
	under := " "
	if !atEnd {
		under = string(window[caretAt])
	}
	val := window[:caretAt] + term.Caret(under)
	if !atEnd {
		val += window[caretAt+1:]
	}
	return val
}

// renderActionRow renders the centered button row, the focused button a
// self-contained sunset bar, the rest a global cool ramp.
func renderActionRow(st *formState, cols int) string {
	const gap = 3
	labels := st.spec.Actions

	actionW := 0
	for i, l := range labels {
		actionW += len(l) + 4 // "[ " .. " ]"
		if i+1 < len(labels) {
			actionW += gap
		}
	}

	var actions strings.Builder
	actionCol := 0
	for i, l := range labels {
		sel := st.onActionRow() && i == st.actionCursor
		button := "[ " + l + " ]"
		if sel {
			actions.WriteString(term.NeonBar(button, len(button), 0))
		} else {
			actions.WriteString(term.CoolGradient(button, actionW, actionCol))
		}
		actionCol += len(button)
		if i+1 < len(labels) {
			actions.WriteString(strings.Repeat(" ", gap))
			actionCol += gap
		}
	}

	// Center the whole button row within cols, so it sits under the middle of the
	// centered field block.
	actionIndent := 0
	if cols > actionW {
		actionIndent = (cols - actionW) / 2
	}
	return "\n" + strings.Repeat(" ", actionIndent) + actions.String() + "\n\n"
}

// renderStatusLine renders the bottom hint/status line.
func renderStatusLine(st *formState, cols int) string {
	switch {
	case st.status != "":
		return centerWrapped(st.status, cols, term.Accent) + "\n"
	case st.editing:
		return centerWrapped("Type to edit · ←/→ move · Enter confirm · Esc cancel", cols, term.Muted) + "\n"
	case !st.onActionRow() && st.fields[st.cursor].Kind == FieldPath:
		// Path field: Enter browses with the OS folder picker, 'e' text-edits.
		return centerWrapped("Up/Down move · Enter browse · e edit · Tab next · Esc cancel", cols, term.Muted) + "\n"
	default:
		return centerWrapped("Up/Down move · Left/Right change · Enter edit/confirm · Tab next · Esc cancel", cols, term.Muted) + "\n"
	}
}

// centerWrapped word-wraps a plain single-line message to cols visible columns,
// then styles and centers each resulting line, joined by newlines (no trailing
// newline). Wrapping is what keeps the footer/hint — the widest lines in the form —
// from spilling past the content box when the host window is narrower than the
// form's natural width: at cols≥natural nothing wraps (term.Wrap only breaks on a
// real overflow), so a wide window is unchanged; a narrow one folds the "·"-list
// onto a second row instead of overrunning the right margin. Styling is applied
// AFTER wrapping so the SGR escapes don't throw off the width measure.
func centerWrapped(text string, cols int, style func(string) string) string {
	lines := term.Wrap(text, cols)
	for i, line := range lines {
		lines[i] = term.Center(style(line), cols)
	}
	return strings.Join(lines, "\n")
}

// verticallyCenter centers the content within winRows by padding it to fill the
// WHOLE window — blank rows above AND below — so the frame is exactly winRows
// tall. Filling every row is what removes the scrollbar: a WT alt screen shows a
// scrollbar whenever the emitted content is shorter than the window (the unpainted
// bottom rows read as scrollback); emitting a row for every window line (each then
// painted by OnPage) makes the buffer equal the window. The gap is split slightly
// top-biased (the worker's proven 2/5) so the content sits a touch above center.
// winRows is the height to fill: the form passes its snapped grid height (a live
// query can lag an async resize), modals pass the live size.
func verticallyCenter(out string, winRows int) string {
	// Visible rows the content occupies. A frame without a trailing newline still
	// fills one more row than it has '\n's, so +1.
	contentRows := countLines(out) + 1
	slack := winRows - contentRows
	if slack <= 0 {
		return out // content fills (or overflows) the window → nothing to pad
	}
	topPad := (slack * 2) / 5
	botPad := slack - topPad
	var b strings.Builder
	b.WriteString(strings.Repeat("\n", topPad))
	b.WriteString(out)
	// out has no trailing newline, so each bottom blank row needs its own leading
	// newline; botPad rows means botPad newlines after the content's last row.
	b.WriteString(strings.Repeat("\n", botPad))
	return b.String()
}
