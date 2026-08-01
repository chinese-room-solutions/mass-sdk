package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/term"
	"github.com/stretchr/testify/require"
)

// colOf is the column text starts at in row. Only meaningful with styling off (a
// plain row's byte index IS its column), which is how these tests measure layout.
func colOf(t *testing.T, row, text string) int {
	t.Helper()
	i := strings.Index(row, text)
	require.GreaterOrEqual(t, i, 0, "%q is not in %q", text, row)
	return i
}

// leadCol is the column a row's first non-blank cell sits at.
func leadCol(row string) int {
	return len(row) - len(strings.TrimLeft(row, " "))
}

// installSummary is the shape a real install leaves: a couple of paths that fit
// their rows whole, and one launch line long enough that the room right of an
// axis-anchored gutter cannot hold it.
func installSummary() term.Summary {
	return term.Summary{
		Kind:     term.SummaryOK,
		Headline: "MASS v0.1.0 installed",
		Rows: []term.SummaryRow{
			{Label: "installed", Value: "/home/op/.local/lib/mass"},
			{Label: "data", Value: "/home/op/.local/share/mass"},
			{Label: "launch", Value: "your applications menu, or `mass` from a terminal"},
		},
	}
}

// summaryGeometry rebuilds the layout SummaryRows hands the menu element — both
// columns sized from the summary's own content — and the geometry it is placed
// with (the content-sized block centred on the band, value column capped to the
// room the block leaves).
func summaryGeometry(s term.Summary, cols int) (MenuLayout, MenuGeometry) {
	rowW := min(cols, term.ContentWidth) - 1
	l := MenuLayout{Gap: summaryGap, MinValueCol: 8}
	for _, r := range s.Rows {
		l.LabelCol = max(l.LabelCol, term.VisibleWidth(r.Label))
		l.ValueCol = max(l.ValueCol, term.VisibleWidth(r.Value))
	}
	return l, summaryGeometryFor(l, rowW)
}

// The summary is not a lookalike of the form's grid — it IS that grid, rendered
// and PLACED by the same element, so a row is compared against what RenderMenuRow
// produces at the element's own geometry. What the summary changes is only the
// layout it hands the element: its own two columns, its own gutter, no selector.
func TestSummaryRowsComeOutOfTheMenuElement(t *testing.T) {
	forcePlainCaps(t)

	for _, cols := range []int{90, 200} {
		t.Run(fmt.Sprintf("%d cols", cols), func(t *testing.T) {
			rowW := min(cols, term.ContentWidth) - 1
			s := installSummary()
			layout, geo := summaryGeometry(s, cols)

			rows := SummaryRows(s, cols)
			require.Equal(t,
				padTo(RenderMenuRow(MenuRow{
					Left: "installed", Right: s.Rows[0].Value, Style: MenuRowStatic,
				}, layout, geo), rowW),
				rows[2],
				"the row IS the menu element's output, padded to the band")

			labelCol := colOf(t, rows[2], "installed")
			valueCol := colOf(t, rows[2], s.Rows[0].Value)
			require.Equal(t, geo.Indent, labelCol, "the label column is where the element placed the block")
			require.Equal(t, geo.Indent+geo.ValueStart, valueCol, "the value column is the element's ValueStart")
			require.Equal(t, layout.LabelCol+summaryGap, valueCol-labelCol,
				"the gutter between the two columns is the summary's own snug gap")
			for i, row := range rows {
				require.Equal(t, rowW, term.VisibleWidth(row),
					"row %d is padded to the band, so the modal centres it by zero", i)
			}
		})
	}
}

// The block's placement: columns hug their content and the whole block is centred
// as a unit on the band — the seam lands wherever the content puts it, not on the
// axis, and the widest value gets the room it needs.
func TestSummaryRowsCenterTheBlock(t *testing.T) {
	forcePlainCaps(t)

	for _, cols := range []int{90, 200} {
		t.Run(fmt.Sprintf("%d cols", cols), func(t *testing.T) {
			rowW := min(cols, term.ContentWidth) - 1
			s := installSummary()
			layout, geo := summaryGeometry(s, cols)

			rows := SummaryRows(s, cols)
			require.Equal(t, layout.LabelCol+layout.Gap+layout.ValueCol, geo.BlockW,
				"both columns hug their content — nothing stretches to the band")
			require.Equal(t, (rowW-geo.BlockW)/2, leadCol(rows[2]), "the block is centred as a unit")
			require.Equal(t, geo.Indent, leadCol(rows[2]))

			for i, row := range rows[2:] {
				require.LessOrEqual(t, len(strings.TrimRight(row, " ")), rowW,
					"row %d ends inside the band's rows", i)
			}
		})
	}
}

// Every value the band can hold stays whole — including the launch line the old
// seam-on-axis placement folded at every width; only a value wider than the room
// the centred block can give it folds onto continuation rows, losslessly.
func TestSummaryRowsFoldOnlyWhatPassesTheRoom(t *testing.T) {
	forcePlainCaps(t)
	const cols = 90

	s := installSummary()
	_, geo := summaryGeometry(s, cols)
	require.GreaterOrEqual(t, geo.ValueWidth, term.VisibleWidth(s.Rows[2].Value),
		"the centred block gives the launch line its full width")

	rows := SummaryRows(s, cols)
	require.Len(t, rows, 1+1+3, "headline + blank + three facts, nothing folded")

	valueCol := leadCol(rows[2]) + geo.ValueStart
	for i, r := range s.Rows {
		require.Equal(t, r.Value, strings.TrimRight(rows[2+i][valueCol:], " "),
			"value %d stays on one row, whole", i)
	}

	// A value wider than the band still folds, on its separators, losslessly.
	long := term.Summary{Kind: term.SummaryOK, Headline: "x", Rows: []term.SummaryRow{
		{Label: "data", Value: "/home/a-user-with-a-really-long-name/.local/share/mass/deeply/nested/dir/of/things"},
	}}
	_, lgeo := summaryGeometry(long, cols)
	lrows := SummaryRows(long, cols)
	require.Greater(t, len(lrows), 3, "an over-band value folds")
	lvalue := lgeo.Indent + lgeo.ValueStart
	var back strings.Builder
	for i, row := range lrows[2:] {
		want := lvalue // a continuation's label cell is blank: it leads at the value column
		if i == 0 {
			want = lgeo.Indent // the first row leads with the label
		}
		require.Equal(t, want, leadCol(row), "row %d off its column", i)
		back.WriteString(strings.TrimRight(row[lvalue:], " "))
	}
	require.Equal(t, long.Rows[0].Value, back.String(), "the folded value reads back whole")

	for i, row := range append(rows, lrows...) {
		require.NotContains(t, row, "…", "row %d was ellipsized", i)
	}
}

// wrapValue folds a value into a width-wide column losslessly: prose on its
// spaces, an unbreakable path just past the last separator that fits (its own
// components, not mid-name), and only a piece with no separator breaks at the
// column edge.
func TestWrapValue(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        []string
	}{
		{
			name:  "folds on the last separator that fits",
			value: "/home/a-user-with-a-really-long-name/.local/share/grimoire/vaults/notes",
			want:  []string{"/home/a-user-with-a-really-long-name/", ".local/share/grimoire/vaults/notes"},
		},
		{
			name:  "a windows path folds on its own separator",
			value: `C:\Users\a-user-with-a-really-long-name\AppData\Local\Grimoire\vaults`,
			want:  []string{`C:\Users\a-user-with-a-really-long-name\`, `AppData\Local\Grimoire\vaults`},
		},
		{
			name:  "nothing to fold on: the column edge",
			value: strings.Repeat("x", 90),
			want:  []string{strings.Repeat("x", 40), strings.Repeat("x", 40), strings.Repeat("x", 10)},
		},
		{
			name:  "prose folds on its spaces",
			value: "your applications menu, or `mass` from a terminal",
			want:  []string{"your applications menu, or `mass` from a", "terminal"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const width = 40
			got := wrapValue(tc.value, width)
			require.Equal(t, tc.want, got)
			sep := ""
			if strings.Contains(tc.value, " ") {
				sep = " "
			}
			require.Equal(t, tc.value, strings.Join(got, sep), "the rows spell the value back, whole")
			for i, p := range got {
				require.LessOrEqual(t, len([]rune(p)), width, "piece %d overflows the column", i)
			}
		})
	}
}

// A cramped terminal degrades like the form does: the element floors the value
// column at MinValueCol, everything folds under it, and every row still starts on
// the label column or the value column with nothing clipped.
func TestSummaryRowsDegradeOnACrampedTerminal(t *testing.T) {
	forcePlainCaps(t)
	const cols = 40

	s := installSummary()
	_, geo := summaryGeometry(s, cols)
	rows := SummaryRows(s, cols)

	labelCol, valueCol := leadCol(rows[2]), leadCol(rows[2])+geo.ValueStart
	var rebuilt []string
	for i, row := range rows[2:] {
		require.NotContains(t, row, "…", "row %d was ellipsized", i)
		require.LessOrEqual(t, len(strings.TrimRight(row, " ")), cols-1,
			"row %d stays inside the band", i)
		require.Contains(t, []int{labelCol, valueCol}, leadCol(row),
			"row %d starts on the label column or the value column, nothing else", i)
		rebuilt = append(rebuilt, strings.TrimRight(row[valueCol:], " "))
	}
	require.Equal(t, s.Rows[0].Value, strings.Join(rebuilt[:len(wrapValue(s.Rows[0].Value, geo.ValueWidth))], ""),
		"the first path reads back whole across its folds")
}

// The headline is the one row NOT on the grid: it is centered on the band's axis,
// over the table, whatever the rows under it hold.
func TestSummaryRowsCenterTheHeadline(t *testing.T) {
	t.Cleanup(term.ForceCaps(true, false))

	s := installSummary()
	rows := SummaryRows(s, 200)
	band := min(200, term.ContentWidth)
	require.Equal(t, (band-term.VisibleWidth(s.Head()))/2, leadCol(rows[0]))
	require.NotEqual(t, leadCol(rows[0]), leadCol(rows[2]), "not padded with the block")
}

// A summary with no headline is nothing to show — the same contract Summary.Lines
// holds to, so neither face has to special-case it.
func TestSummaryRowsWithoutAHeadlineRenderNothing(t *testing.T) {
	forcePlainCaps(t)
	require.Nil(t, SummaryRows(term.Summary{}, 90))
}
