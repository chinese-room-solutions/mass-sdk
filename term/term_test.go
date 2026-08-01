package term

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// withCaps overrides the styling/truecolor probes for one test and restores them
// after, so the pure transforms can be exercised in every mode without a TTY.
func withCaps(t *testing.T, styling, truecolor bool) {
	t.Helper()
	t.Cleanup(ForceCaps(styling, truecolor))
}

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"plain ascii", "hello", 5},
		{"empty", "", 0},
		{"sgr wrapped", "\033[1mhi\033[0m", 2},
		{"truecolor wrapped", "\033[38;2;1;2;3mX\033[0m", 1},
		{"multibyte glyph", "✔", 1},
		{"mixed", "\033[36ma\033[0mb✔", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, VisibleWidth(tt.in))
		})
	}
}

func TestCenter(t *testing.T) {
	withCaps(t, true, false)
	tests := []struct {
		name  string
		line  string
		width int
		want  string
	}{
		{"centers plain", "ab", 6, "  ab"},
		{"already wider", "abcdef", 4, "abcdef"},
		{"zero width no-op", "ab", 0, "ab"},
		{"centers by visible width", "\033[1mab\033[0m", 6, "  \033[1mab\033[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Center(tt.line, tt.width))
		})
	}
}

// The content band is centered in the window: no shift until the window is wider
// than the band, then half the surplus. 0/negative (unknown width) means "don't
// center", so the block stays flush left.
func TestContentOrigin(t *testing.T) {
	tests := []struct {
		name string
		cols int
		want int
	}{
		{"wide window centers the band", 200, 54}, // (200 - 92) / 2
		{"exactly the band", ContentWidth, 0},
		{"half a column over: no shift", ContentWidth + 1, 0},
		{"narrower than the band", 80, 0},
		{"unknown width", 0, 0},
		{"negative width", -1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ContentOrigin(tt.cols))
		})
	}
}

// IndentBlock shifts a whole composed block, non-empty lines only: a blank row
// stays blank, so a block ending in a newline leaves the cursor at column 0 and a
// leading newline still puts its payload at the indent (not the spaces on the
// previous row). n<=0 is the "unknown width" pass-through.
func TestIndentBlock(t *testing.T) {
	withCaps(t, true, false)
	tests := []struct {
		name  string
		block string
		n     int
		want  string
	}{
		{"shifts every line", "a\nb", 2, "  a\n  b"},
		{"leaves blank rows blank", "a\n\nb\n", 2, "  a\n\n  b\n"},
		{"a leading newline keeps its payload indented", "\nb", 2, "\n  b"},
		{"empty block", "", 2, ""},
		{"zero is a pass-through", "a\nb", 0, "a\nb"},
		{"negative is a pass-through", "a\nb", -3, "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IndentBlock(tt.block, tt.n))
		})
	}
}

// A non-styling stream (a pipe, a captured child console) is never indented at
// all, so its output stays flush left.
func TestIndentBlockUnstyledIsNoop(t *testing.T) {
	withCaps(t, false, false)
	require.Equal(t, "a\nb", IndentBlock("a\nb", 4))
}

func TestCenterUnstyledIsNoop(t *testing.T) {
	withCaps(t, false, false)
	require.Equal(t, "ab", Center("ab", 80))
}

func TestRGBTruecolor(t *testing.T) {
	withCaps(t, true, true)
	require.Equal(t, "\033[38;2;10;20;30mX\033[0m", RGB(10, 20, 30, "X"))
}

func TestRGB16Color(t *testing.T) {
	withCaps(t, true, false)
	// cyan-ish (B&G,!R) → ANSI 36
	require.Equal(t, "\033[36mX\033[0m", RGB(0, 240, 255, "X"))
	// magenta (R&B,!G) → 35
	require.Equal(t, "\033[35mX\033[0m", RGB(255, 40, 160, "X"))
}

func TestRGBUnstyled(t *testing.T) {
	withCaps(t, false, false)
	require.Equal(t, "X", RGB(1, 2, 3, "X"))
}

func TestNearestANSIFG(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b uint8
		want    string
	}{
		{"magenta", 255, 0, 255, "35"},
		{"cyan", 0, 255, 255, "36"},
		{"blue", 0, 0, 255, "34"},
		{"green", 0, 255, 0, "32"},
		{"yellow", 255, 255, 0, "33"},
		{"red", 255, 0, 0, "31"},
		{"white", 255, 255, 255, "37"},
		{"black defaults cyan", 0, 0, 0, "36"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, nearestANSIFG(tt.r, tt.g, tt.b))
		})
	}
}

func TestGradientEndpoints(t *testing.T) {
	// sunset: t=0 → magenta start, t=1 → gold end.
	require.Equal(t, rgbColor{255, 40, 160}, sunsetRGB(0))
	require.Equal(t, rgbColor{255, 210, 90}, sunsetRGB(1))
	// the segment seam at t=0.5 is coral.
	require.Equal(t, rgbColor{255, 110, 70}, sunsetRGB(0.5))
	// cool: t=0 → electric cyan, t=1 → deep blue.
	require.Equal(t, rgbColor{0, 240, 255}, coolRGB(0))
	require.Equal(t, rgbColor{70, 110, 235}, coolRGB(1))
}

func TestProgressBar(t *testing.T) {
	withCaps(t, false, false) // unstyled → deterministic ASCII
	require.Equal(t, "[###-------] 3/10", ProgressBar(3, 10, 10))
	require.Equal(t, "[----------] 0/10", ProgressBar(0, 10, 10))
	require.Equal(t, "[##########] 10/10", ProgressBar(10, 10, 10))
	// clamps overflow and avoids /0.
	require.Equal(t, "[##########] 5/5", ProgressBar(9, 5, 10))
	require.Equal(t, "[----------] 0/1", ProgressBar(0, 0, 10))
}

func TestOnPageUnstyledNoop(t *testing.T) {
	withCaps(t, false, false)
	require.Equal(t, "a\nb", OnPage("a\nb", 80))
}

func TestOnPagePadsToWidthMinusOne(t *testing.T) {
	withCaps(t, true, true)
	out := OnPage("ab", 6)
	// bg + "ab" + 3 spaces (fill = width-1 = 5, visible 2) + reset.
	require.Equal(t, "\033[48;2;20;12;40mab   \033[0m", out)
}

func TestBannerUnstyledIsPlainArt(t *testing.T) {
	withCaps(t, false, false)
	art := []string{"MA", "SS"}
	out := Banner(art, "[ mass ]", 0)
	require.Equal(t, "MA\nSS\n[ mass ]\n\n", out)
}

func TestHeading(t *testing.T) {
	withCaps(t, false, false)
	require.Equal(t, "\nTitle\n-----\n", Heading("Title"))
}

func TestMarksUnstyled(t *testing.T) {
	withCaps(t, false, false)
	require.Equal(t, "ok ", OKMark())
	require.Equal(t, "x ", FailMark())
	require.Equal(t, "- ", NoteMark())
}

func TestCaret(t *testing.T) {
	withCaps(t, false, false)
	require.Equal(t, "[x]", Caret("x"))
	withCaps(t, true, true)
	require.Equal(t, "\033[1;38;2;235;230;245;48;2;120;96;170mx\033[0m", Caret("x"))
}

// Wrap word-wraps plain text: breaks on spaces at the column boundary, never
// mid-word; leaves an overlong word intact; honours existing '\n' as hard breaks;
// and with width<=0 splits only on '\n'. Ported from the worker's term_test.cpp.
func TestWrap(t *testing.T) {
	const url = "/var/lib/mass-worker-llama-with-a-very-long-name"
	cases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"breaks on spaces at width", "one two three four", 8, []string{"one two", "three", "four"}},
		{"does not split an overlong word", url, 10, []string{url}},
		{"wraps words around an overlong word", "see " + url + " here", 10, []string{"see", url, "here"}},
		{"honours hard breaks", "first line.\nsecond line.", 40, []string{"first line.", "second line."}},
		{"hard break holds when a segment also wraps", "aa bb cc\ndd ee", 5, []string{"aa bb", "cc", "dd ee"}},
		{"non-positive width splits only on newlines", "a very long line with many words", 0, []string{"a very long line with many words"}},
		{"non-positive width still honours newlines", "one\ntwo", -1, []string{"one", "two"}},
		{"boundary is inclusive: exact width fits", "abcd efgh", 9, []string{"abcd efgh"}},
		{"one over the boundary spills the last word", "abcd efgh", 8, []string{"abcd", "efgh"}},
		// "·" (U+00B7) is 2 bytes but 1 visible column: measured by width, "a · b" is
		// 5 columns and fits at width 5, rather than its 6-byte length forcing a wrap.
		{"multibyte separator counts as one column", "a · b", 5, []string{"a · b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, Wrap(tc.text, tc.width))
		})
	}
}
