//go:build linux

package term

import "golang.org/x/sys/unix"

// The ioctl request that fetches termios on this OS, used only as an isatty
// probe (a successful get means stdout is a terminal).
const termiosReq = unix.TCGETS
