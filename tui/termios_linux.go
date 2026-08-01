//go:build linux

package tui

import "golang.org/x/sys/unix"

// termios ioctl requests on Linux.
const (
	termiosGet      = unix.TCGETS  // tcgetattr
	termiosSet      = unix.TCSETS  // tcsetattr TCSANOW
	termiosSetFlush = unix.TCSETSF // tcsetattr TCSAFLUSH
	termiosFlush    = unix.TCFLSH  // tcflush
)
