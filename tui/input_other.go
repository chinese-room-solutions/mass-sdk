//go:build !windows

package tui

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// rawModeImpl owns the /dev/tty file and the saved termios to restore.
type rawModeImpl struct {
	tty   *os.File
	saved *unix.Termios
}

// enterRawMode opens the controlling terminal directly (so we read keys even
// when stdin is a pipe — a double-click launcher — and aren't fooled by sudo's
// stdin desync) and switches it to raw mode: canonical mode off (byte-at-a-time),
// echo off, signal generation off (Ctrl-C/Ctrl-Z arrive as bytes).
func enterRawMode() (rawModeImpl, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return rawModeImpl{}, ErrNotATTY
	}
	fd := int(tty.Fd())
	saved, err := unix.IoctlGetTermios(fd, termiosGet)
	if err != nil {
		tty.Close() //nolint:errcheck // best-effort cleanup on the error path
		return rawModeImpl{}, ErrNotATTY
	}
	raw := *saved
	raw.Lflag &^= unix.ICANON | unix.ECHO | unix.ISIG | unix.IEXTEN
	raw.Iflag &^= unix.IXON | unix.ICRNL | unix.BRKINT | unix.INPCK | unix.ISTRIP
	raw.Cc[unix.VMIN] = 1 // block for at least one byte
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, termiosSetFlush, &raw); err != nil {
		tty.Close() //nolint:errcheck // best-effort cleanup on the error path
		return rawModeImpl{}, ErrRawModeFailed
	}
	return rawModeImpl{tty: tty, saved: saved}, nil
}

func (r *rawModeImpl) close() {
	if r.tty == nil {
		return
	}
	if r.saved != nil {
		unix.IoctlSetTermios(int(r.tty.Fd()), termiosSetFlush, r.saved) //nolint:errcheck // best-effort restore of the saved mode
	}
	r.tty.Close() //nolint:errcheck // read side only; nothing actionable
	r.tty = nil
}

func (r *rawModeImpl) discardPending() {
	if r.tty != nil {
		// Drop bytes queued before now (TCIFLUSH).
		unix.IoctlSetInt(int(r.tty.Fd()), termiosFlush, unix.TCIFLUSH) //nolint:errcheck // best-effort discard
	}
}

func (r *rawModeImpl) readKey() (Key, error) {
	if r.tty == nil {
		return Key{}, fmt.Errorf("tui: readKey after close: %w", ErrReadFailed)
	}
	fd := int(r.tty.Fd())
	var buf [1]byte
	n, err := unix.Read(fd, buf[:])
	if n == 0 {
		return Key{Type: KeyEof}, nil
	}
	if err != nil || n < 0 {
		return Key{}, fmt.Errorf("tui: read: %w", ErrReadFailed)
	}

	// Continuation reads for an escape sequence use a brief timeout so a lone ESC
	// (no bytes follow) is reported as KeyEsc rather than blocking forever. Switch
	// the terminal to a polling mode for just these reads, then restore VMIN=1.
	next := func() (byte, bool) {
		poll, err := unix.IoctlGetTermios(fd, termiosGet)
		if err != nil {
			return 0, false
		}
		blocking := *poll
		poll.Cc[unix.VMIN] = 0
		poll.Cc[unix.VTIME] = 1 // 0.1s — long enough for a terminal's CSI burst
		unix.IoctlSetTermios(fd, termiosSet, poll) //nolint:errcheck // best-effort poll mode; a failed switch just blocks as before
		var b [1]byte
		rn, _ := unix.Read(fd, b[:])
		unix.IoctlSetTermios(fd, termiosSet, &blocking) //nolint:errcheck // best-effort restore
		if rn == 1 {
			return b[0], true
		}
		return 0, false
	}
	return parseKey(buf[0], next), nil
}
