//go:build windows

package tui

import (
	"os"

	"golang.org/x/sys/windows"
)

// termSizeOS returns the console rows+cols, or the 80x24 fallback.
func termSizeOS() termSize {
	fd := windows.Handle(os.Stdout.Fd())
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(fd, &info); err != nil {
		return termSize{rows: 24, cols: 80}
	}
	return termSize{
		rows: int(info.Window.Bottom-info.Window.Top) + 1,
		cols: int(info.Window.Right-info.Window.Left) + 1,
	}
}
