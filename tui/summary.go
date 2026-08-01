package tui

import (
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/term"
)

// summaryGap is the gutter between a summary's label and value columns. Narrower
// than the form's term.Gutter on purpose: the form's seam-on-axis placement gave
// the values only the room right of the axis, and every launch line folded. The
// summary is a read-back table, not a field grid — snug columns, whole values.
const summaryGap = 4

// SummaryRows renders a finished action's summary through the SAME menu element the
// form's field rows go through — RenderMenuRow draws every row — so the result
// screen is the two-column grid the operator was just reading, not a lookalike
// built by hand. It comes back ready for BackOrExit.
//
// The element is given a layout of the summary's own size: the label column as wide
// as the widest label, the value column as wide as the widest VALUE, summaryGap
// between them, and no selector marker — nothing here is selectable. The block is
// centred as a unit on the band, so the columns hug their content and a value folds
// only when the band genuinely can't hold the widest one (then it folds below,
// losslessly, floored at MinValueCol). This deliberately departs from the form's
// gutter-on-axis rule, which capped the value room at half a band and folded the
// launch line at every width.
//
// The rows come back right-padded to the band, so BackOrExit's align+centre
// pipeline is a no-op on them and they land exactly where they were put; only the
// headline is centred, on the band's axis over them. cols is the terminal width;
// the band derived from it matches the one runModal composes within.
func SummaryRows(s term.Summary, cols int) []string {
	if s.Headline == "" {
		return nil
	}
	band := max(0, min(cols, term.ContentWidth))
	// One column shy of the band — the cell term.OnPage leaves empty so a row can't
	// write into the window's last column and arm the terminal's pending-wrap flag.
	// Still wide enough that Center adds no pad of its own ((band−(band−1))/2 == 0).
	rowW := max(0, band-1)

	// Marker 0: nothing here is selectable. MinValueCol keeps a readable floor
	// when a cramped terminal squeezes the room right of the gutter.
	layout := MenuLayout{Gap: summaryGap, MinValueCol: 8}
	for _, r := range s.Rows {
		layout.LabelCol = max(layout.LabelCol, term.VisibleWidth(r.Label))
		layout.ValueCol = max(layout.ValueCol, term.VisibleWidth(r.Value))
	}
	geo := summaryGeometryFor(layout, rowW)

	// A blank row under the headline: the title stands off its table. Padded like
	// every row so the modal's centring stays a no-op.
	rows := []string{padTo(term.Center(s.Head(), band), rowW), padTo("", rowW)}
	for _, r := range s.Rows {
		// A value wider than the column folds into continuation rows: same static row
		// with an empty label, which leaves the label cell blank and starts the text
		// at the value column. Folding is a multi-ROW job, so it belongs here, where
		// the rows are assembled, rather than inside an element that renders one.
		for i, piece := range wrapValue(r.Value, geo.ValueWidth) {
			label := r.Label
			if i > 0 {
				label = ""
			}
			row := MenuRow{Left: label, Right: piece, Style: MenuRowStatic}
			rows = append(rows, padTo(RenderMenuRow(row, layout, geo), rowW))
		}
	}
	return rows
}

// summaryGeometryFor places a summary's content-sized block: the value column is
// capped to the room the block leaves right of the gutter (floored at
// MinValueCol), and the whole block is centred on the axis width. The form's
// MenuGeometryFor instead hangs the GUTTER's midpoint on the axis — right for a
// field grid the eye scans down, wrong for this table, where it halves the value
// room and folds values the band could hold whole.
func summaryGeometryFor(layout MenuLayout, cols int) MenuGeometry {
	fixed := layout.LabelCol + layout.Gap
	valueWidth := layout.ValueCol
	if room := cols - fixed; valueWidth > room {
		valueWidth = max(room, layout.MinValueCol)
	}
	blockW := fixed + valueWidth
	return MenuGeometry{
		Indent:     max(0, (cols-blockW)/2),
		ValueWidth: valueWidth,
		BlockW:     blockW,
		ValueStart: fixed,
	}
}

// wrapValue word-wraps a summary value into a width-wide value column, and BREAKS a
// piece that still doesn't fit. term.Wrap leaves an over-long word over-long, which
// is right for prose — but the values here are mostly paths, a single word with no
// space to break on, and a row wider than the band spills out of the modal's frame
// to wrap at the window's own edge. Values reach here unstyled, so a rune is a cell.
//
// Such a break lands on the last path separator that fits, kept on the row it ends,
// so a path folds on its own components ("…/.local/lib/" + "grimoire") instead of
// mid-name ("…/lib/grimoi" + "re"); only a piece with no separator to fold on breaks
// at the column edge. Nothing is ever clipped or dropped — every rune of the value
// reaches a row.
func wrapValue(value string, width int) []string {
	pieces := term.Wrap(value, width)
	if width <= 0 {
		return pieces
	}
	out := make([]string, 0, len(pieces))
	for _, piece := range pieces {
		rs := []rune(piece)
		for len(rs) > width {
			cut := breakAt(rs, width)
			out = append(out, string(rs[:cut]))
			rs = rs[cut:]
		}
		out = append(out, string(rs))
	}
	return out
}

// breakAt is where to cut rs, which is longer than width cells, to fill one row: just
// past the last path separator within width, else at width. Index 0 doesn't count — a
// row holding only a leading separator is no better than the edge break. Both
// separators are honoured whatever the host OS, since the values are its paths.
func breakAt(rs []rune, width int) int {
	for i := width - 1; i > 0; i-- {
		if rs[i] == '/' || rs[i] == '\\' {
			return i + 1
		}
	}
	return width
}

// padTo right-pads row with spaces to width visible columns (a row already that wide
// is returned unchanged).
func padTo(row string, width int) string {
	if w := term.VisibleWidth(row); w < width {
		return row + strings.Repeat(" ", width-w)
	}
	return row
}
