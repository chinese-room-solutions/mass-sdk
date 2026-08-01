package term

import "strings"

// --- What survives the exit --------------------------------------------------
//
// An installer draws its wizard on the alternate screen, and every byte of it is
// gone when that screen is restored — on purpose: the operator's terminal comes
// back exactly as they left it. So the Summary is the only trace of what
// happened, and it has to carry what they still need: what was done, where it
// landed, and how to use it.

// SummaryKind is how a finished action reads.
type SummaryKind uint8

const (
	// SummaryOK is a completed action (✔ + the cool ramp).
	SummaryOK SummaryKind = iota
	// SummaryNote is an outcome with nothing to celebrate — nothing changed, or
	// the work moved elsewhere (• + muted).
	SummaryNote
	// SummaryFail is a failed action (✖ + accent).
	SummaryFail
)

// SummaryRow is one label → value pair under the headline.
type SummaryRow struct{ Label, Value string }

// Summary is what a finished action leaves behind: one headline plus the ordered
// facts under it. Build it once per action and render it with Lines, so the
// in-session result screen and the trace left on the restored terminal can't
// drift apart.
type Summary struct {
	Kind     SummaryKind
	Headline string
	Rows     []SummaryRow
}

// Head is the summary's headline behind the kind's mark, styled: the one row that
// goes over the table. Built here, from the kind, so the in-session result screen
// and the trace left on the restored terminal can't read differently — they lay the
// rows out for their own frame, but the headline is the same row in both.
func (s Summary) Head() string {
	switch s.Kind {
	case SummaryNote:
		return NoteMark() + Muted(s.Headline)
	case SummaryFail:
		return FailMark() + Accent(s.Headline)
	default:
		return OKMark() + Cool(s.Headline)
	}
}

// Lines renders the summary: the label→value rows as two columns a Gutter apart —
// every label padded to the widest one plus that gap, so the values line up too —
// under the kind's mark + headline centered over them, a title over its table.
// Styled through this package's helpers, so a NO_COLOR or piped run gets the same
// text in plain ASCII. A summary with no headline renders as nothing.
func (s Summary) Lines() []string {
	if s.Headline == "" {
		return nil
	}
	labels := 0
	for _, r := range s.Rows {
		labels = max(labels, len([]rune(r.Label)))
	}

	rows := make([]string, 0, len(s.Rows))
	block := 0 // widest rendered row: the headline's centering axis
	for _, r := range s.Rows {
		label := r.Label + strings.Repeat(" ", labels-len([]rune(r.Label))+Gutter)
		row := Muted(label) + Cool(r.Value)
		block = max(block, VisibleWidth(row))
		rows = append(rows, row)
	}

	head := s.Head()
	if w := VisibleWidth(head); w < block {
		head = strings.Repeat(" ", (block-w)/2) + head
	}

	return append([]string{head}, rows...)
}
