package tui

import (
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/term"
	"github.com/stretchr/testify/require"
)

// stubNoRawMode makes raw-mode entry fail for the test, as on a non-tty.
func stubNoRawMode(t *testing.T) {
	t.Helper()
	orig := enterRawModeFn
	enterRawModeFn = func() (*RawMode, error) { return nil, ErrNotATTY }
	t.Cleanup(func() { enterRawModeFn = orig })
}

// When the environment can't host the screen, the modal helpers signal
// ErrDeclined — the caller's cue to fall back to a plain linear prompt. This is
// distinct from a user cancel, which resolves to a button choice.
func TestModalsDeclineWithoutRawMode(t *testing.T) {
	stubNoRawMode(t)

	yes, err := Confirm(nil, "", []string{"Continue?"}, true)
	require.ErrorIs(t, err, ErrDeclined)
	require.False(t, yes)

	back, err := BackOrExit(nil, "", []string{"done"})
	require.ErrorIs(t, err, ErrDeclined)
	require.False(t, back)

	back, err = ErrorScreen(nil, "", "boom")
	require.ErrorIs(t, err, ErrDeclined)
	require.False(t, back)
}

// A modal renders the way runModal draws it: composed within the content band,
// then shifted to the band's column. On a 200-column window that puts the whole
// block 54 columns in — centered on the window's axis, not anchored at its left
// edge — while the banner stays centered over the message under it (the band's
// axis, not the window's).
func TestComposeModalShiftsTheBandOntoTheWindowAxis(t *testing.T) {
	t.Cleanup(term.ForceCaps(true, false))

	const win = 200
	band := min(win, term.ContentWidth)
	origin := term.ContentOrigin(win)
	require.Equal(t, 92, band)
	require.Equal(t, 54, origin)

	spec := ModalSpec{
		BannerArt: []string{"ABCD"},
		Tag:       "[ t ]",
		Lines:     []ModalLine{{Text: "Install now.", Kind: ModalProse}},
		Buttons:   [2]string{"Yes", "No"},
		Footer:    "Enter selects",
	}

	// Every non-blank row as (leading spaces, visible width of its content):
	// centered in the band and shifted, i.e. origin + (band − width) / 2.
	want := [][2]int{
		{98, 4},  // "ABCD"             — the wordmark art
		{97, 5},  // "[ t ]"            — the tag
		{94, 12}, // "Install now."     — the message
		{92, 16}, // "[ Yes ]   [ No ]" — the buttons
		{93, 13}, // "Enter selects"    — the footer
	}

	var got [][2]int
	block := term.IndentBlock(composeModal(spec, 0, band), origin)
	for _, row := range strings.Split(block, "\n") {
		lead := strings.IndexFunc(row, func(r rune) bool { return r != ' ' })
		if lead < 0 {
			continue // blank
		}
		got = append(got, [2]int{lead, term.VisibleWidth(row) - lead})
	}
	require.Equal(t, want, got)
}

// A styled block (a summary of marks + label→value rows) is padded to one width
// before it is centered, so the rows keep a single left edge instead of each
// centering on its own length.
func TestAlignBlockPadsToTheWidestLine(t *testing.T) {
	require.Equal(t,
		[]string{"ok done  ", "installed", "data     "},
		alignBlock([]string{"ok done", "installed", "data"}))
	require.Equal(t, []string{}, alignBlock(nil))
}
