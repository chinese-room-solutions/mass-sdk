package tui

import (
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/term"
)

// A reusable three-column terminal menu: label | gutter | value, each a FIXED
// width. It renders the same synthwave grid the setup form uses — a global cool
// ramp across the unselected rows, a self-contained sunset bar on the selected
// one — but knows nothing about what a "field" means, so any menu that wants
// aligned label/value rows can drive it.
//
// Two properties define the layout:
//   - The selector marker ("> ") lives INSIDE the label column (it overwrites the
//     column's first cells) rather than as a prefix, so selecting a row never
//     changes the menu's width or shifts the columns.
//   - Rows are placed so the GUTTER's midpoint lands on the centering axis. With
//     asymmetric columns (label ≠ value) that is NOT the same as centering the
//     whole block — centering the block would push the gap off-axis. Anchoring the
//     gutter keeps the gap dead-centre whatever any label or value holds.
//
// Pure (no tty, no rendering side effects) so it is unit-testable and drops into
// any caller that already uses the term package. Presentation only.
//
// This is a Go port of the worker's C++ menu component
// (mass-worker-llama-cpp: include/mass_worker/menu.hpp + src/menu.cpp). The two
// are independent copies (different languages, no shared source), so a change to
// the layout rules or the kMenuLayout widths here should be mirrored there, and
// vice versa, to keep both installers' setup forms visually identical.

// MenuLayout is the fixed geometry of the three columns, in character cells.
// MinValueCol is the floor the value column collapses to when the axis is too
// narrow to hold the full menu (a cramped terminal), so the columns degrade
// instead of spilling.
type MenuLayout struct {
	LabelCol    int // label column width (marker occupies its head)
	Gap         int // the gutter column width
	ValueCol    int // value column width (values clip to it with "…")
	MinValueCol int // floor for ValueCol on a too-narrow axis
	Marker      int // width of the "> " / "  " selector, drawn IN LabelCol
}

// MenuRowStyle is a row's visual state. Selected → the sunset bar (the focused
// row); Normal → the cool grid ramp shared across the unselected rows; Static → a
// fact read back rather than a row to move onto (see MenuRowStatic).
type MenuRowStyle uint8

const (
	MenuRowNormal MenuRowStyle = iota
	MenuRowSelected
	// MenuRowStatic is a row nobody can move onto: a finished action's summary read
	// back in the grid's own two columns. It reads as a caption, not as one of an
	// interactive ramp — muted label, cool value — and its value is NOT clipped to
	// the value column: a summary is the only trace left once the alternate screen
	// goes, so it may not lose characters, and the caller has already folded it to
	// geo.ValueWidth (a value over that width spills, exactly as a caller would
	// deserve). Continuation rows are ordinary static rows with an empty Left, which
	// leaves the label cell blank and starts the text at the value column.
	MenuRowStatic
)

// MenuRow is one menu row's content. Left is the label text WITHOUT the marker
// (the renderer adds "> " / "  "); Right is the value text already in its final
// display form (e.g. secrets pre-masked, choices pre-bracketed) — the menu clips
// it to the value column but does not otherwise interpret it. IsChoice only
// tweaks the styling (the cool ramp treats a bracketed choice value as one span);
// it does not change the layout.
type MenuRow struct {
	Left     string
	Right    string
	Style    MenuRowStyle
	IsChoice bool
}

// MenuGeometry is the resolved geometry for a given axis width cols (0 → no
// centering, the menu is left-flush at its intrinsic width — used to MEASURE the
// menu's natural box). ValueWidth is the value column after any narrow-axis
// shrink; BlockW is the full menu width (LabelCol + Gap + ValueWidth);
// ValueStart is the column the value begins at (LabelCol + Gap), which callers
// doing their own in-row drawing (e.g. an inline editor) need to place the value.
type MenuGeometry struct {
	Indent     int // leading spaces to the menu's left edge
	ValueWidth int // resolved value column
	BlockW     int // full menu width
	ValueStart int // column where the value column begins
}

// MenuGeometryFor computes the geometry that anchors the gutter's midpoint on
// cols/2. Pure.
//
// The value column is also capped so the block's RIGHT edge never passes cols:
// the narrow-axis shrink triggers on the menu's left-flush width, but the block
// is placed by its gutter, and a menu whose label column is much narrower than
// its value column sits far enough right that it would spill before any shrink
// fires. (The C++ menu component carries the same cap.)
func MenuGeometryFor(layout MenuLayout, cols int) MenuGeometry {
	fixed := layout.LabelCol + layout.Gap // before the value

	// The value column shrinks below ValueCol only when the axis can't hold the
	// whole menu, and never below MinValueCol — a cramped terminal degrades instead
	// of spilling.
	valueWidth := layout.ValueCol
	if cols > 0 && cols < fixed+layout.ValueCol {
		if cols > fixed+layout.MinValueCol {
			valueWidth = cols - fixed
		} else {
			valueWidth = layout.MinValueCol
		}
	}

	// Anchor the gutter's midpoint on cols/2. The gutter's centre sits
	// LabelCol + Gap/2 from the menu's left edge, so the menu starts that many
	// cells before the axis. (Centering the whole block would put the gap off-axis
	// whenever LabelCol ≠ ValueWidth.)
	indent := 0
	if cols > 0 {
		gutterCenter := layout.LabelCol + layout.Gap/2
		if half := cols / 2; half > gutterCenter {
			indent = half - gutterCenter
		}

		// The shrink above measures the menu LEFT-FLUSH, but the block is placed by
		// its gutter: cap the value column to the room the placed block actually
		// has (never below the floor), or the right edge passes cols.
		if room := cols - indent - fixed; valueWidth > room {
			valueWidth = max(room, layout.MinValueCol)
		}
	}

	return MenuGeometry{
		Indent:     indent,
		ValueWidth: valueWidth,
		BlockW:     fixed + valueWidth,
		ValueStart: fixed,
	}
}

// MenuLabelCell is the left column ONLY (marker + left padded to LabelCol, then
// the gutter), as plain text — no value, no gradient. Exposed so a caller that
// draws its own value in the value column (e.g. an inline editor with a live
// caret) builds the label column by the SAME rule the menu uses, keeping its
// columns aligned with rendered rows. selected picks the "> " marker over the
// blank one. Pure.
func MenuLabelCell(left string, layout MenuLayout, selected bool) string {
	// Marker + label, clipped to LabelCol (with "…") then padded, then the gutter.
	// The marker is part of the column (it overwrites its head), so the column stays
	// exactly LabelCol wide regardless of selection or label length — an over-long
	// label clips instead of overflowing and shoving the value column right.
	marker := strings.Repeat(" ", layout.Marker)
	if selected {
		marker = "> "
	}
	cell := marker + clip(left, layout.LabelCol-layout.Marker)
	if len(cell) < layout.LabelCol {
		cell += strings.Repeat(" ", layout.LabelCol-len(cell))
	}
	return cell + strings.Repeat(" ", layout.Gap)
}

// RenderMenuRow renders one row to its styled string (no trailing newline), laid
// out at geo. The marker is added from row.Style; the value is clipped to
// geo.ValueWidth with "…", except on a MenuRowStatic row, which shows it whole.
// Pure.
func RenderMenuRow(row MenuRow, layout MenuLayout, geo MenuGeometry) string {
	label := MenuLabelCell(row.Left, layout, row.Style == MenuRowSelected)

	// The indent goes BEFORE any styling so a highlight bar can't bleed into the
	// centering margin.
	out := strings.Repeat(" ", geo.Indent)
	if row.Style == MenuRowStatic {
		// No ramp: the ramp is what ties the interactive rows together, and this row
		// is not one of them. No clip either — see MenuRowStatic.
		return out + term.Muted(label) + term.Cool(row.Right)
	}
	if row.Style == MenuRowSelected {
		// Self-contained sunset gradient spanning the bar's own width. A choice still
		// shows its "< >" brackets (drawn plain inside the bar) so a selected choice
		// reads the same as an unselected one.
		value := clip(row.Right, geo.ValueWidth)
		if row.IsChoice {
			value = "< " + value + " >"
		}
		bar := label + value
		return out + term.NeonBar(bar, term.VisibleWidth(bar), 0)
	}

	// Unselected rows share ONE cool ramp keyed by column across the whole menu
	// width, so the body reads as a single cohesive grid rather than a per-row ramp:
	// the label from column 0, the value from ValueStart. A choice keeps its value on
	// the ramp but wraps it in neon-pink "< >" brackets.
	val := term.CoolGradient(clip(row.Right, geo.ValueWidth), geo.BlockW, geo.ValueStart)
	if row.IsChoice {
		val = term.Accent("< ") + val + term.Accent(" >")
	}
	return out + term.CoolGradient(label, geo.BlockW, 0) + val
}
