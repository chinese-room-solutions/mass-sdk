package term

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// widestLinePad returns the column the widest visible line of a printed page starts
// at. On a phase page that line is the widest line of the banner art, so this is
// where the banner — composed at the full window width — put it.
func widestLinePad(t *testing.T, page string) int {
	t.Helper()
	pad, widest := 0, 0
	for _, line := range strings.Split(page, "\n") {
		body := strings.TrimLeft(line, " ")
		if w := VisibleWidth(body); w > widest {
			pad, widest = len(line)-len(body), w
		}
	}
	require.Positive(t, widest, "the page printed nothing to measure")
	return pad
}

// centered is row preceded by the pad that centers it in a cols-wide window — the
// contract every row of a phase page holds to, applied to that row's OWN width.
func centered(row string, cols int) string {
	return strings.Repeat(" ", (cols-VisibleWidth(row))/2) + row
}

// A step row must always start on a clean row: whatever a phase prints after a
// live transient row (a spinner frame, a progress bar) has to be preceded by the
// '\r' + erase that releases it, or it lands on top of the bar and smears.
func TestPhaseEndsTheTransientRowBeforePrinting(t *testing.T) {
	withCaps(t, true, false)
	const release = "\r\033[K"

	tests := []struct {
		name string
		// setup pairs "draw a transient row" with "print over it".
		setup func(p *Phase) (live, over func())
	}{
		{"line after a bar", func(p *Phase) (func(), func()) {
			return func() { p.Progress(3, 8) }, func() { p.Line("done") }
		}},
		{"heading after a bar", func(p *Phase) (func(), func()) {
			return func() { p.Progress(3, 8) }, func() { p.Heading("Next") }
		}},
		{"line after a spinner frame", func(p *Phase) (func(), func()) {
			var s *Spinner
			return func() { s = p.Spinner("working"); s.Tick() }, func() { p.Line("done") }
		}},
		{"spinner result replaces its own frame", func(p *Phase) (func(), func()) {
			var s *Spinner
			return func() { s = p.Spinner("working") }, func() { s.Finish(true) }
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			p := NewPhase(&buf)
			live, over := tt.setup(p)

			live()
			before := buf.String()
			require.NotEmpty(t, before, "the transient row must have been drawn")

			over()
			printed := strings.TrimPrefix(buf.String(), before)
			require.True(t, strings.HasPrefix(printed, release),
				"a print over a live transient row must release it first, got %q", printed)
			require.NotContains(t, printed[len(release):], release,
				"the row is released once, not per line")
		})
	}
}

// A transient row is REDRAWN in place, so it erases before it repaints: two frames
// of different width — a bar whose count grew a digit, a centered row whose pad
// shifted — must not leave the wider one's tail on the row.
func TestPhaseTransientRowErasesBeforeItRedraws(t *testing.T) {
	withCaps(t, true, false)

	var buf strings.Builder
	p := NewPhase(&buf)
	p.Progress(9, 22)
	buf.Reset()
	p.Progress(10, 22)
	require.True(t, strings.HasPrefix(buf.String(), "\r\033[K"),
		"a redraw starts by erasing the row it replaces, got %q", buf.String())
}

// Close is the safety net for an early return: it releases a row left live, and
// does nothing when there is none (so it can be deferred unconditionally).
func TestPhaseCloseReleasesOnlyALiveRow(t *testing.T) {
	withCaps(t, true, false)

	t.Run("live row", func(t *testing.T) {
		var buf strings.Builder
		p := NewPhase(&buf)
		p.Progress(1, 4)
		buf.Reset()
		p.Close()
		require.Equal(t, "\r\033[K", buf.String())
	})

	t.Run("nothing live", func(t *testing.T) {
		var buf strings.Builder
		p := NewPhase(&buf)
		p.Line("step")
		buf.Reset()
		p.Close()
		p.Close()
		require.Empty(t, buf.String(), "no live row, nothing to release")
	})
}

// The page face centers EVERY row on the window axis by that row's own visible
// width — the heading and its rule, each step row whatever its length, and the
// progress bar. No shared left edge: rows of different widths get different pads.
func TestPhasePageCentersEachRowOnItsOwnWidth(t *testing.T) {
	// Two art lines so the banner's own centering is measured against the widest.
	art := []string{"ART", "ARTWORK"}
	const narrow, wide = 90, 200 // narrower and wider than ContentWidth

	tests := []struct {
		name  string
		print func(p *Phase)
		want  func(cols int) string
	}{
		{"heading and its rule share one width", func(p *Phase) { p.Heading("Installing") },
			func(cols int) string {
				return "\n" + centered(Bold(Cyan("Installing")), cols) + "\n" +
					centered(Dim("----------"), cols) + "\n"
			}},
		{"a short step row", func(p *Phase) { p.Line("step") },
			func(cols int) string { return centered("step", cols) + "\n" }},
		{"a longer step row gets its own pad", func(p *Phase) { p.Line(OKMark() + "Staged the files") },
			func(cols int) string { return centered(OKMark()+"Staged the files", cols) + "\n" }},
		{"a row that opens with a blank one", func(p *Phase) { p.Line("\nclosing") },
			func(cols int) string { return "\n" + centered("closing", cols) + "\n" }},
		{"the progress bar centers on its own width", func(p *Phase) { p.Progress(1, 2) },
			func(cols int) string {
				return "\r\033[K" + centered(ProgressBar(1, 2, progressBarWidth), cols)
			}},
	}

	for _, cols := range []int{narrow, wide} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%d cols: %s", cols, tt.name), func(t *testing.T) {
				withCaps(t, true, true)
				var buf strings.Builder
				p := OpenPhasePage(&buf, art, "[ v1 ]", cols)
				require.Equal(t, (cols-artWidth(art))/2, widestLinePad(t, buf.String()),
					"the banner still composes at the full window width")

				buf.Reset()
				tt.print(p)
				require.Equal(t, tt.want(cols), buf.String())
			})
		}
	}
}

// The centering belongs to the page: the scripted face and the dumb-terminal
// fallback open none, so their rows print flush at column 0 whatever the window.
func TestPhaseWithoutAPagePrintsFlush(t *testing.T) {
	t.Run("no page, no centering", func(t *testing.T) {
		withCaps(t, true, true)
		var buf strings.Builder
		p := NewPhase(&buf)
		p.Line("step")
		p.Progress(1, 2)
		require.Equal(t, "step\n\r\033[K"+ProgressBar(1, 2, progressBarWidth), buf.String())
	})

	t.Run("unstyled opens no page", func(t *testing.T) {
		withCaps(t, false, false)
		var buf strings.Builder
		p := OpenPhasePage(&buf, []string{"ART"}, "[ v1 ]", 200)
		require.Empty(t, buf.String(), "no clear, no banner, nothing to spray into a log")

		p.Line("step")
		require.Equal(t, "step\n", buf.String())
	})
}

// On a non-styling stream nothing rewrites a row: the bar is dropped entirely and
// the spinner degrades to one plain label line plus its result, so a log gets rows
// instead of carriage returns.
func TestPhaseUnstyledDrawsNoTransientRows(t *testing.T) {
	withCaps(t, false, false)

	var buf strings.Builder
	p := NewPhase(&buf)
	p.Progress(3, 8)
	require.Empty(t, buf.String(), "a bar in a log is a smear of carriage returns")

	s := p.Spinner("working")
	s.Tick()
	s.Finish(true)
	require.Equal(t, "working...\nok working\n", buf.String())
	require.NotContains(t, buf.String(), "\r")
}
