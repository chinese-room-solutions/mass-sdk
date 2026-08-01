package tui

import (
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/term"
	"github.com/stretchr/testify/require"
)

var testMenuLayout = MenuLayout{Marker: 2, LabelCol: 25, Gap: 12, ValueCol: 30, MinValueCol: 16}

// MenuGeometryFor anchors the gutter's midpoint on cols/2, shrinks the value
// column only on a too-narrow axis (never below the floor), and reports 0 indent
// for cols=0 (measure mode, left-flush).
func TestMenuGeometry(t *testing.T) {
	l := testMenuLayout
	gutterCenter := l.LabelCol + l.Gap/2 // 25 + 6 = 31

	cases := []struct {
		name       string
		cols       int
		wantIndent int
		wantValueW int
	}{
		{"measure mode (cols=0), left-flush", 0, 0, l.ValueCol},
		{"wide axis: gutter on cols/2, full value", 100, 100/2 - gutterCenter, l.ValueCol},
		// Placing the gutter on the axis leaves less room right of it than the
		// left-flush width suggests, so the placed-room cap trims the value column.
		{"block-width axis: gutter centred, cap trims the spill", l.LabelCol + l.Gap + l.ValueCol, (l.LabelCol+l.Gap+l.ValueCol)/2 - gutterCenter, 28},
		{"narrow axis: value shrinks toward floor", l.LabelCol + l.Gap + 20, 0, 20},
		{"too narrow: value pinned to floor", 30, 0, l.MinValueCol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			geo := MenuGeometryFor(l, tc.cols)
			require.Equal(t, tc.wantIndent, geo.Indent, "indent")
			require.Equal(t, tc.wantValueW, geo.ValueWidth, "value width")
			require.Equal(t, l.LabelCol+l.Gap+geo.ValueWidth, geo.BlockW, "block width")
			require.Equal(t, l.LabelCol+l.Gap, geo.ValueStart, "value start")

			// The gutter's midpoint lands on cols/2 whenever the axis is wide enough to
			// indent (the whole point of the anchor).
			if geo.Indent > 0 {
				require.Equal(t, tc.cols/2, geo.Indent+gutterCenter, "gutter midpoint on cols/2")
			}
		})
	}
}

// A menu whose label column is much narrower than its value column is placed far
// enough right that its edge would pass the axis while the left-flush shrink still
// sees room — the cap trims the value column to the room the PLACED block has,
// and the floor still outranks the fit on a hopeless axis.
func TestMenuGeometryCapsThePlacedBlock(t *testing.T) {
	lopsided := MenuLayout{LabelCol: 9, Gap: 3, ValueCol: 49, MinValueCol: 8}

	geo := MenuGeometryFor(lopsided, 89)
	require.Equal(t, 34, geo.Indent, "gutter midpoint on the axis (44 - 10)")
	require.Equal(t, 43, geo.ValueWidth, "capped to the placed room (89 - 34 - 12)")
	require.LessOrEqual(t, geo.Indent+geo.BlockW, 89, "the right edge stays inside the axis")

	// The floor wins over the fit: a hopeless axis keeps MinValueCol readable (the
	// block spills right rather than collapsing to nothing).
	floor := MenuLayout{LabelCol: 9, Gap: 3, ValueCol: 49, MinValueCol: 30}
	require.Equal(t, 30, MenuGeometryFor(floor, 50).ValueWidth)
}

// MenuLabelCell is always LabelCol+Gap wide and the marker lives INSIDE the label
// column, so selecting a row never changes the menu's width or shifts the columns.
func TestMenuLabelCell(t *testing.T) {
	l := testMenuLayout
	want := l.LabelCol + l.Gap

	for _, selected := range []bool{false, true} {
		cell := MenuLabelCell("Scope", l, selected)
		require.Len(t, cell, want, "label cell is LabelCol+Gap wide (selected=%v)", selected)
	}
	require.True(t, strings.HasPrefix(MenuLabelCell("Scope", l, true), "> "), "selected marker")
	require.True(t, strings.HasPrefix(MenuLabelCell("Scope", l, false), "  "), "unselected marker is blank")

	// A label longer than the column clips to it with "…" — the cell stays EXACTLY
	// LabelCol+Gap columns wide (measured by visible width: "…" is one column but
	// three bytes) so it can never shove the value column right (the bug this guards
	// against: an over-long label overflowing and mis-aligning its value).
	long := strings.Repeat("x", l.LabelCol+5)
	cell := MenuLabelCell(long, l, false)
	require.Equal(t, want, term.VisibleWidth(cell), "an over-long label still fits exactly LabelCol+Gap columns")
	require.Contains(t, cell, "…", "an over-long label clips with an ellipsis")
	require.True(t, strings.HasSuffix(cell, strings.Repeat(" ", l.Gap)), "gutter follows the clipped label")
}

// A long value clips to the value column with an ellipsis; a choice value is
// wrapped in "< >" brackets. (Styling is off in tests, so the plain layout shows.)
func TestRenderMenuRowClipsAndBrackets(t *testing.T) {
	l := testMenuLayout
	geo := MenuGeometryFor(l, 100)

	long := strings.Repeat("y", geo.ValueWidth+10)
	row := RenderMenuRow(MenuRow{Left: "Dir", Right: long}, l, geo)
	require.Contains(t, row, "…", "an over-long value clips with an ellipsis")

	choice := RenderMenuRow(MenuRow{Left: "Scope", Right: "User", IsChoice: true}, l, geo)
	require.Contains(t, choice, "< User >", "a choice value is bracketed")
}

// The SELECTED choice row clips its value to the value column exactly like
// every other row — it used to rebuild the bracketed value from the raw text,
// letting an over-long choice spill past the menu's width.
func TestRenderMenuRowSelectedChoiceClips(t *testing.T) {
	forcePlainCaps(t)
	l := testMenuLayout
	geo := MenuGeometryFor(l, 100)

	long := strings.Repeat("z", geo.ValueWidth+10)
	row := RenderMenuRow(MenuRow{Left: "Backend", Right: long, IsChoice: true, Style: MenuRowSelected}, l, geo)

	require.Contains(t, row, "…", "an over-long selected choice clips with an ellipsis")
	// Brackets survive around the CLIPPED value, and the row never exceeds the
	// menu block (plus the 4 bracket cells) — the same envelope an unselected
	// choice row occupies.
	require.Contains(t, row, "< ")
	require.Contains(t, row, " >")
	require.LessOrEqual(t, term.VisibleWidth(row), geo.Indent+geo.BlockW+4,
		"selected choice must not spill past the menu block")

	// A selected and an unselected choice row render the same visible width for
	// the same content (selection must never change the layout).
	unselected := RenderMenuRow(MenuRow{Left: "Backend", Right: long, IsChoice: true}, l, geo)
	require.Equal(t, term.VisibleWidth(unselected), term.VisibleWidth(row))
}

// A STATIC row is the summary's row: no marker, no ramp, and — unlike a field —
// no clip. A summary is the only trace of the run once the alternate screen goes,
// so the element must hand back every character it was given and leave the folding
// to the caller that assembles the rows. Its continuation carries an empty label,
// which leaves the cell blank and starts the text on the value column.
func TestRenderMenuRowStaticShowsItsValueWhole(t *testing.T) {
	forcePlainCaps(t)
	// A summary's layout: its own label column, its own gutter, and Marker 0 —
	// nothing here is selectable, so the cell has no selector to leave room for.
	l := MenuLayout{LabelCol: 9, Gap: 3, ValueCol: 30}
	geo := MenuGeometryFor(l, 100)

	long := strings.Repeat("z", geo.ValueWidth+10)
	row := RenderMenuRow(MenuRow{Left: "installed", Right: long, Style: MenuRowStatic}, l, geo)

	require.NotContains(t, row, "…", "a static row never ellipsizes its value")
	require.Contains(t, row, long, "the value comes back whole")
	require.Equal(t, geo.Indent, colOf(t, row, "installed"), "no selector marker is drawn")
	require.Equal(t, geo.Indent+geo.ValueStart, colOf(t, row, long),
		"the value starts on the element's value column")

	cont := RenderMenuRow(MenuRow{Left: "", Right: "tail", Style: MenuRowStatic}, l, geo)
	require.Equal(t, geo.Indent+geo.ValueStart, colOf(t, cont, "tail"),
		"an empty label leaves the cell blank and joins the value column")
}
