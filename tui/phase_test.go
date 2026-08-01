package tui

import (
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/term"
	"github.com/stretchr/testify/require"
)

// captureSink redirects the package's output sink for one test and returns it.
func captureSink(t *testing.T) *strings.Builder {
	t.Helper()
	var buf strings.Builder
	old := stdout
	stdout = &buf
	t.Cleanup(func() { stdout = old })
	return &buf
}

// Everything the phase page draws — the clear above all — has to sit between the
// alt-screen enter and leave. RunForm's screen is already gone when an action runs,
// so a clear outside that bracket would erase the operator's own terminal.
func TestOpenPhaseBracketsEveryClearInTheAltScreen(t *testing.T) {
	t.Cleanup(term.ForceCaps(true, true))
	buf := captureSink(t)

	ph, closePhase := OpenPhase([]string{"ART"}, "[ v1 ]", 120)
	ph.Line("step")
	closePhase()

	out := buf.String()
	enter := strings.Index(out, "\033[?1049h")
	leave := strings.Index(out, "\033[?1049l")
	require.GreaterOrEqual(t, enter, 0, "the page must enter the alt screen")
	require.Greater(t, leave, enter, "and leave it once it closes")
	require.Greater(t, strings.Index(out, "step"), enter, "the rows draw on that screen")
	require.Less(t, strings.Index(out, "step"), leave)

	// Every erase-display, wherever it appears, lands inside the bracket.
	for _, clear := range []string{"\033[2J", "\033[3J"} {
		for at, rest := 0, out; ; {
			i := strings.Index(rest, clear)
			if i < 0 {
				break
			}
			at += i
			require.Greater(t, at, enter, "a clear before the alt screen wipes the operator's terminal")
			require.Less(t, at, leave, "a clear after the leave wipes the operator's terminal")
			at += len(clear)
			rest = out[at:]
		}
	}

	// Idempotent: a deferred close after an explicit one must not leave twice.
	closePhase()
	require.Equal(t, 1, strings.Count(buf.String(), "\033[?1049l"))
}

// With styling off there is no screen to open: the dumb-terminal fallback keeps the
// transcript it printed and the rows join it at column 0.
func TestOpenPhaseUnstyledOpensNoScreen(t *testing.T) {
	t.Cleanup(term.ForceCaps(false, false))
	buf := captureSink(t)

	ph, closePhase := OpenPhase([]string{"ART"}, "[ v1 ]", 120)
	ph.Line("step")
	closePhase()

	require.Equal(t, "step\n", buf.String())
}
