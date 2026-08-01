//go:build !windows

package tui

import (
	"os"

	"golang.org/x/sys/unix"
)

// termSizeOS returns the terminal rows+cols, or the 80x24 fallback. Prefers the
// controlling terminal: under an AppImage/konsole "-e" launch, stdout may not be
// the tty, so TIOCGWINSZ on stdout fails and we'd wrongly fall back (breaking
// centering). /dev/tty is the real terminal.
func termSizeOS() termSize {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0); err == nil {
		defer tty.Close() //nolint:errcheck // read-only probe
		if ws, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ); err == nil && ws.Row > 0 {
			return termSize{rows: int(ws.Row), cols: int(ws.Col)}
		}
	}
	if ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err == nil && ws.Row > 0 {
		return termSize{rows: int(ws.Row), cols: int(ws.Col)}
	}
	return termSize{rows: 24, cols: 80}
}
