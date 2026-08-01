package tui

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/term"
	"github.com/stretchr/testify/require"
)

// captureScreen redirects both output sinks the screen helpers write to — the
// package's stdout var (writeRaw) and os.Stdout (the term package's escapes) —
// runs fn with styling+truecolor forced on, and returns everything written.
func captureScreen(t *testing.T, fn func()) string {
	t.Helper()
	t.Cleanup(term.ForceCaps(true, true))

	var buf strings.Builder
	oldSink := stdout
	stdout = &buf
	t.Cleanup(func() { stdout = oldSink })

	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStdout := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = oldStdout
	require.NoError(t, w.Close())
	termOut, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return buf.String() + string(termOut)
}

func TestLeaveResetsTerminalBG(t *testing.T) {
	out := captureScreen(t, func() { altScreen{active: true}.leave() })
	require.Contains(t, out, "\033[?1049l", "must leave the alt buffer")
	require.Contains(t, out, "\033]111\007", "must reset the OSC 11 background")
}

func TestLeaveInactiveEmitsNothing(t *testing.T) {
	out := captureScreen(t, func() { altScreen{}.leave() })
	require.Empty(t, out)
}

// A held session refcounts the buffer switch: views entering and leaving inside
// it neither re-enter nor restore the terminal, so the operator's PRIMARY screen
// (and any text selection sitting on it) never flashes back mid-wizard. The
// restore happens exactly once, at release — and release is idempotent.
func TestHoldScreenSpansNestedViews(t *testing.T) {
	out := captureScreen(t, func() {
		release := HoldScreen()
		view := enterAltScreen(0, 0)
		view.leave()
		view2 := enterAltScreen(0, 0)
		view2.leave()
		release()
		release()
	})
	require.Equal(t, 1, strings.Count(out, "\033[?1049h"), "entered more than once")
	require.Equal(t, 1, strings.Count(out, "\033[?1049l"), "left more than once")
	require.Less(t, strings.Index(out, "\033[?1049h"), strings.Index(out, "\033[?1049l"))
	require.Equal(t, 0, screenDepth, "session accounting must return to zero")
}

func TestSuspendScreenKeepsThemedBG(t *testing.T) {
	out := captureScreen(t, SuspendScreen)
	require.Contains(t, out, "\033[?1049l", "must leave the alt buffer")
	require.Contains(t, out, "\033[?25h", "must re-show the cursor")
	require.Contains(t, out, "\033]11;", "must re-assert the OSC 11 background")
	require.NotContains(t, out, "\033]111\007", "hand-off keeps the themed page")
}

func TestResumeScreenReentersAltScreen(t *testing.T) {
	out := captureScreen(t, ResumeScreen)
	require.Contains(t, out, "\033[?1049h", "must re-enter the alt buffer")
	require.Contains(t, out, "\033[?25l", "must hide the cursor")
	require.Contains(t, out, "\033]11;", "must re-assert the OSC 11 background")
}

func TestSuspendResumeNoopWhenUnstyled(t *testing.T) {
	t.Cleanup(term.ForceCaps(false, false))
	var buf strings.Builder
	oldSink := stdout
	stdout = &buf
	t.Cleanup(func() { stdout = oldSink })

	SuspendScreen()
	ResumeScreen()
	require.Empty(t, buf.String())
}
