//go:build !linux && !windows

package tui

import "golang.org/x/sys/unix"

// termios ioctl requests on Darwin/BSD.
const (
	termiosGet      = unix.TIOCGETA // tcgetattr
	termiosSet      = unix.TIOCSETA // tcsetattr TCSANOW
	termiosSetFlush = unix.TIOCSETAF // tcsetattr TCSAFLUSH
	termiosFlush    = unix.TIOCFLUSH // tcflush
)
