//go:build !windows

package term

import (
	"os"

	"golang.org/x/sys/unix"
)

// detectStyling: a TTY check is enough on POSIX; virtually every terminal there
// understands SGR.
func detectStyling() bool {
	if _, ok := getEnv("NO_COLOR"); ok {
		return false
	}
	_, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), termiosReq)
	return err == nil
}

// detectTruecolorOS: off Windows, without COLORTERM we assume only 16-colour
// (the caller already checked COLORTERM). Matches the worker's conservative
// default — a terminal that supports truecolor virtually always advertises it
// via COLORTERM.
func detectTruecolorOS() bool { return false }

// terminalWidthOS returns the terminal width in columns, or 0 when it can't be
// determined. Prefers the controlling terminal: under an AppImage/konsole "-e"
// launch, stdout may not be the tty, so TIOCGWINSZ on stdout fails and we'd
// wrongly return 0 (disabling centering). /dev/tty is the real terminal.
func terminalWidthOS() int {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0); err == nil {
		defer tty.Close() //nolint:errcheck // read-only probe
		if ws, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ); err == nil && ws.Col > 0 {
			return int(ws.Col)
		}
	}
	if ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err == nil && ws.Col > 0 {
		return int(ws.Col)
	}
	return 0
}
