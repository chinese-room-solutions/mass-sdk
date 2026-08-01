//go:build windows

package term

import (
	"os"

	"golang.org/x/sys/windows"
)

// detectStyling probes capability and, as a side effect, flips the console into
// VT-processing mode — modern terminals (Windows Terminal, recent conhost)
// accept it; older ones return an error and we fall back to plain text.
func detectStyling() bool {
	if _, ok := getEnv("NO_COLOR"); ok {
		return false
	}
	fd := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(fd, &mode); err != nil {
		return false // redirected to a file/pipe, or not a console
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true // already on
	}
	return windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

// detectTruecolorOS: reaching this at all means StylingEnabled() saw VT
// processing get enabled, which on Windows implies a console new enough
// (conhost 1703+ / Windows Terminal) to do 24-bit color too.
func detectTruecolorOS() bool { return true }

// terminalWidthOS returns the console width in columns, or 0 when it can't be
// determined.
func terminalWidthOS() int {
	fd := windows.Handle(os.Stdout.Fd())
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(fd, &info); err != nil {
		return 0
	}
	return int(info.Window.Right-info.Window.Left) + 1
}
