package install

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDispatcherScript_BakesSizeAndHasNoPlaceholders(t *testing.T) {
	s := ContainerSpec{Cols: 100, Rows: 30}
	got := s.dispatcherScript()
	require.Contains(t, got, "cols=100; rows=30")
	require.NotContains(t, got, "__COLS__")
	require.NotContains(t, got, "__ROWS__")
}

func TestDispatcherScript_DefaultsWhenZero(t *testing.T) {
	got := ContainerSpec{}.dispatcherScript()
	require.Contains(t, got, "cols=88; rows=26")
	// Forwarded-args and tty branches must be present — they're what keep a sudo
	// re-exec from looping into a new window and let a shell launch run inline.
	require.Contains(t, got, `if [ "$#" -gt 0 ]; then exec "$app" "$@"; fi`)
	require.Contains(t, got, "Darwin") // the macOS Terminal.app branch
}

// The macOS branch runs the app and closes its window, but does NOT size it: the
// wizard sizes itself via the CSI 8t escape (Terminal.app honours it), so driving
// the size from AppleScript here is both unnecessary and unreliable. Guard that we
// don't reintroduce a window-resize call — and that a throwaway .terminal profile
// (which would pollute the user's Terminal preferences) isn't written either.
func TestDispatcherScript_MacDoesNotSizeWindow(t *testing.T) {
	got := ContainerSpec{Cols: 88, Rows: 26}.dispatcherScript()
	require.Contains(t, got, `do script cmd`, "must run the app")
	require.Contains(t, got, `close win saving no`, "must close the app's window")
	require.NotContains(t, got, "number of columns of", "must not resize the window from AppleScript")
	require.NotContains(t, got, "number of rows of", "must not resize the window from AppleScript")
	require.NotContains(t, got, "Window Settings", "must not write a throwaway .terminal profile (pollutes prefs)")
}

// A cold Terminal.app launch auto-opens a blank window; a bare `do script` then
// spawns a second — two windows. The dispatcher gates on Terminal's pre-launch
// running state (captured before it touches Terminal) and, when it was NOT running,
// runs the wizard IN that blank front window rather than opening another, so exactly
// one window remains — the double-terminal fix. Reusing a blindly-chosen `window 1`
// would be wrong (it can be a user's shell), so the reuse is guarded by `wasRunning`.
func TestDispatcherScript_MacReusesColdLaunchBlankWindow(t *testing.T) {
	got := ContainerSpec{Cols: 88, Rows: 26}.dispatcherScript()
	require.Contains(t, got, `set wasRunning to application \"Terminal\" is running`,
		"must snapshot whether Terminal was already running, before launching it")
	require.Contains(t, got, `do script cmd in front window`,
		"on a cold launch must run the wizard in the blank window Terminal just opened, not a second one")
	require.Contains(t, got, `if wasRunning then`,
		"the second-window vs reuse choice must be gated on the pre-launch running state")
}

// The residual terminate prompt ("-bash (2), path_helper") came from closing the
// window while its login shell was still sourcing its startup profile. `busy` alone
// flickers false during that gap, so the close must ALSO wait until the tab's process
// list is empty — i.e. the `; exit` has fully unwound the shell — before closing.
func TestDispatcherScript_MacWaitsForShellToFullyExit(t *testing.T) {
	got := ContainerSpec{Cols: 88, Rows: 26}.dispatcherScript()
	require.Contains(t, got, `count of (processes of w)) is 0`,
		"must close only once the tab's shell has fully exited, not merely gone idle")
}

func TestBuildContainer_RequiresFields(t *testing.T) {
	_, err := BuildContainer(ContainerSpec{ID: "x", BinPath: "/bin/sh"})
	require.Error(t, err) // missing Name
	_, err = BuildContainer(ContainerSpec{Name: "X", BinPath: "/bin/sh"})
	require.Error(t, err) // missing ID
}

func TestBuildContainer_RejectsMissingBinary(t *testing.T) {
	_, err := BuildContainer(ContainerSpec{
		Name: "X", ID: "x", BinPath: "/nonexistent/binary", OutDir: t.TempDir(),
	})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "not found") ||
		strings.Contains(err.Error(), "not supported"))
}
