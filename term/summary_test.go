package term

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The summary renders as the rows — every label padded to the widest one plus a
// Gutter, so labels share one column and values share the next — under the kind's
// mark + headline, centered over the widest of them. Styling is off, so the marks
// degrade to ASCII and the padding is the only thing shaping the block.
func TestSummaryLines(t *testing.T) {
	withCaps(t, false, false)

	tests := []struct {
		name string
		s    Summary
		want []string
	}{
		{
			name: "ok kind, padded label column, headline centered over the widest row",
			s: Summary{
				Kind:     SummaryOK,
				Headline: "MASS v0.1.0 installed",
				Rows: []SummaryRow{
					{Label: "installed", Value: "/opt/mass"},
					{Label: "data", Value: "/var/lib/mass"},
					{Label: "launch", Value: "your applications menu"},
				},
			},
			// Labels pad to 9 ("installed") + 12, so every value starts at column 21.
			// Widest row is 43 columns, the headline 24 → (43-24)/2 = 9.
			want: []string{
				"         ok MASS v0.1.0 installed",
				"installed            /opt/mass",
				"data                 /var/lib/mass",
				"launch               your applications menu",
			},
		},
		{
			name: "note kind, no rows to center over",
			s:    Summary{Kind: SummaryNote, Headline: "no changes made"},
			want: []string{"- no changes made"},
		},
		{
			name: "fail kind",
			s: Summary{
				Kind:     SummaryFail,
				Headline: "install failed",
				Rows:     []SummaryRow{{Label: "error", Value: "permission denied"}},
			},
			// The row is 5+12+17 = 34 columns, the headline 16 → (34-16)/2 = 9.
			want: []string{"         x install failed", "error            permission denied"},
		},
		{
			name: "a headline wider than its rows stays put, and one row still gets the gutter",
			s: Summary{
				Kind:     SummaryOK,
				Headline: "Grimoire v0.1.0 removed",
				Rows:     []SummaryRow{{Label: "from", Value: "/opt"}},
			},
			// The row is 4+12+4 = 20 columns, under the headline's 26.
			want: []string{"ok Grimoire v0.1.0 removed", "from            /opt"},
		},
		{
			name: "no headline renders nothing",
			s:    Summary{Kind: SummaryOK, Rows: []SummaryRow{{Label: "a", Value: "b"}}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.s.Lines())
		})
	}
}

// Each kind carries its own mark, and the marks are the styled glyphs when the
// terminal can show them.
func TestSummaryMarks(t *testing.T) {
	withCaps(t, true, false)
	for kind, mark := range map[SummaryKind]string{
		SummaryOK:   OKMark(),
		SummaryNote: NoteMark(),
		SummaryFail: FailMark(),
	} {
		lines := Summary{Kind: kind, Headline: "done"}.Lines()
		require.Len(t, lines, 1)
		require.True(t, len(lines[0]) > len(mark) && lines[0][:len(mark)] == mark,
			"kind %d must open with its own mark", kind)
	}
}
