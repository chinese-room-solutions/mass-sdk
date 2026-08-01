//go:build windows

package tui

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// rawModeImpl owns the CONIN$ handle and the saved console mode to restore.
type rawModeImpl struct {
	in        windows.Handle
	savedMode uint32
	open      bool
}

// enterRawMode opens CONIN$ (rather than the std handle, so we read the console
// even when stdin is a redirected pipe) and clears line buffering, echo, and
// Ctrl-C processing so keys arrive raw and Ctrl-C becomes a key record.
func enterRawMode() (rawModeImpl, error) {
	name, _ := windows.UTF16PtrFromString("CONIN$")
	h, err := windows.CreateFile(name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil || h == windows.InvalidHandle {
		return rawModeImpl{}, ErrNotATTY
	}

	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		_ = windows.CloseHandle(h)
		return rawModeImpl{}, ErrNotATTY
	}
	raw := mode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT)
	if err := windows.SetConsoleMode(h, raw); err != nil {
		_ = windows.CloseHandle(h)
		return rawModeImpl{}, ErrRawModeFailed
	}
	return rawModeImpl{in: h, savedMode: mode, open: true}, nil
}

// close restores the console mode and releases the handle. Teardown: a failed
// restore has no recovery path, so the errors are intentionally discarded (the
// AGENTS shutdown-path exemption).
func (r *rawModeImpl) close() {
	if !r.open {
		return
	}
	_ = windows.SetConsoleMode(r.in, r.savedMode)
	_ = windows.CloseHandle(r.in)
	r.open = false
}

func (r *rawModeImpl) discardPending() {
	if r.open {
		flushConsoleInputBuffer(r.in)
	}
}

// readKey reads console input records until a key-down event maps to a Key,
// building Keys directly from the key-event record (no ambiguous byte stream,
// so parseKey is unused on Windows).
func (r *rawModeImpl) readKey() (Key, error) {
	if !r.open {
		return Key{}, fmt.Errorf("tui: readKey after close: %w", ErrReadFailed)
	}
	for {
		var rec inputRecord
		var got uint32
		if err := readConsoleInput(r.in, &rec, 1, &got); err != nil {
			return Key{}, fmt.Errorf("tui: ReadConsoleInput: %w", ErrReadFailed)
		}
		if got == 0 {
			return Key{Type: KeyEof}, nil
		}
		if rec.eventType != keyEvent {
			continue // ignore focus, mouse, buffer-resize records
		}
		ke := rec.asKeyEvent()
		if ke.keyDown == 0 {
			continue // ignore key-up
		}
		ctrl := ke.controlKeyState&(leftCtrlPressed|rightCtrlPressed) != 0
		switch ke.virtualKeyCode {
		case vkUp:
			return Key{Type: KeyUp}, nil
		case vkDown:
			return Key{Type: KeyDown}, nil
		case vkLeft:
			if ctrl {
				return Key{Type: KeyCtrlLeft}, nil
			}
			return Key{Type: KeyLeft}, nil
		case vkRight:
			if ctrl {
				return Key{Type: KeyCtrlRight}, nil
			}
			return Key{Type: KeyRight}, nil
		case vkHome:
			return Key{Type: KeyHome}, nil
		case vkEnd:
			return Key{Type: KeyEnd}, nil
		case vkReturn:
			return Key{Type: KeyEnter}, nil
		case vkEscape:
			return Key{Type: KeyEsc}, nil
		case vkTab:
			return Key{Type: KeyTab}, nil
		case vkBack:
			return Key{Type: KeyBackspace}, nil
		}
		ch := ke.unicodeChar
		if ch == 0 {
			continue // a modifier-only / dead key — wait for the next
		}
		if ch == 0x03 {
			return Key{Type: KeyCtrlC}, nil
		}
		if ch == '\r' || ch == '\n' {
			return Key{Type: KeyEnter}, nil
		}
		// The console is CP_UTF8; for the ASCII/BMP inputs a setup flow takes,
		// returning the low byte is exact. Non-ASCII degrades, not corrupts.
		return Key{Type: KeyChar, Byte: byte(ch & 0xff)}, nil
	}
}
