// Package tui is a dependency-free, arrow-key navigable terminal UI engine for
// interactive console flows — the raw-mode input layer plus a generic single-
// screen form and two-button modals, styled via mass-sdk/term.
//
// This is a Go port of the worker's term_input.cpp + form.cpp
// (mass-worker-llama-cpp); the C++ stays there as the reference, this is
// canonical for the Go apps (MASS, Grimoire). The engine is GENERIC: the caller
// supplies the field list and action labels, so nothing app-specific (a worker's
// GPU backend, a coordinator's listen address) leaks into the package — the
// app-specific field building and result mapping live in each app.
//
// Raw terminal state is the one mutable resource here; it is owned by a RawMode
// value whose Close restores the prior mode (RAII via defer), mirroring the C++
// RAII guard.
package tui

import (
	"errors"
	"fmt"
)

// KeyType is one logical keypress. Arrow keys, Enter, etc. are their own types;
// a data byte is Char with the byte in Key.Byte. Editing is byte-oriented: a
// multi-byte UTF-8 character arrives as consecutive Char events.
type KeyType uint8

const (
	KeyNone      KeyType = iota // nothing read (not produced by a blocking read)
	KeyChar                     // a data byte is in Key.Byte
	KeyEnter                    // CR or LF
	KeyBackspace                // DEL (0x7f) or BS (0x08)
	KeyTab
	KeyEsc // a bare ESC, not the start of a CSI sequence
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyCtrlLeft  // word-left in a text field
	KeyCtrlRight // word-right in a text field
	KeyHome
	KeyEnd
	KeyCtrlC // 0x03 — translated, never raised as a signal
	KeyEof   // the terminal returned end-of-input
	KeyUnknown
)

// Key is one decoded keypress; Byte is valid only when Type == KeyChar.
type Key struct {
	Type KeyType
	Byte byte
}

// Errors returned by the input layer. They are sentinels so callers can branch
// on the cause (e.g. fall back to a linear prompt flow when there's no TTY).
var (
	// ErrNotATTY means there's no controlling terminal to read from — the caller
	// should fall back to a non-interactive flow.
	ErrNotATTY = errors.New("tui: no controlling terminal")
	// ErrRawModeFailed means raw mode was refused by the OS.
	ErrRawModeFailed = errors.New("tui: raw mode unavailable")
	// ErrReadFailed means a read from the terminal failed mid-session.
	ErrReadFailed = errors.New("tui: terminal read failed")
)

// parseKey turns a byte stream into ONE key (the POSIX path; the Windows reader
// builds Keys directly from key-event records and never calls this). first is
// the already-read leading byte; next yields each following byte, or (0,false)
// on timeout/EOF — it is called only to read the continuation bytes of an escape
// sequence. A lone ESC (next → false) is disambiguated from a CSI sequence here,
// which is the subtle part this seam exists to unit-test.
func parseKey(first byte, next func() (byte, bool)) Key {
	switch first {
	case '\r', '\n':
		return Key{Type: KeyEnter}
	case '\t':
		return Key{Type: KeyTab}
	case 0x7f, 0x08: // DEL / BS
		return Key{Type: KeyBackspace}
	case 0x03: // Ctrl-C (raw mode delivers it as a byte, ISIG off)
		return Key{Type: KeyCtrlC}
	}

	if first != 0x1b { // not ESC → an ordinary data byte
		return Key{Type: KeyChar, Byte: first}
	}

	// ESC: the start of a CSI/SS3 sequence, or a bare Escape. The disambiguation
	// is whether more bytes follow promptly — next returns false on the short
	// timeout when nothing does.
	b1, ok := next()
	if !ok {
		return Key{Type: KeyEsc}
	}
	if b1 != '[' && b1 != 'O' {
		// ESC followed by something that isn't a CSI/SS3 introducer (e.g. Alt+key
		// = ESC + char). We don't model modifiers; treat as a bare Esc so the
		// stray byte doesn't leak into a text field.
		return Key{Type: KeyEsc}
	}

	b2, ok := next()
	if !ok {
		return Key{Type: KeyEsc}
	}
	// Mouse reports (the screens enable tracking so drags reach us instead of
	// becoming a terminal selection). Swallow the whole report, or its parameter
	// bytes would type digits into whatever field has focus. SGR encoding
	// (\x1b[<b;x;yM/m) is what we request; X10 (\x1b[M + 3 bytes) is the fallback
	// a terminal without SGR support sends.
	if b1 == '[' && b2 == '<' {
		for {
			p, ok := next()
			if !ok || p == 'M' || p == 'm' {
				break
			}
		}
		return Key{Type: KeyUnknown}
	}
	if b1 == '[' && b2 == 'M' {
		for range 3 {
			if _, ok := next(); !ok {
				break
			}
		}
		return Key{Type: KeyUnknown}
	}
	switch b2 {
	case 'A':
		return Key{Type: KeyUp}
	case 'B':
		return Key{Type: KeyDown}
	case 'C':
		return Key{Type: KeyRight}
	case 'D':
		return Key{Type: KeyLeft}
	case 'H':
		return Key{Type: KeyHome}
	case 'F':
		return Key{Type: KeyEnd}
	}
	// A parameterised sequence, e.g. "\x1b[3~" (Delete) or "\x1b[1;5C"
	// (Ctrl+Right). Collect the parameter bytes through the final letter so the
	// remainder isn't misread as separate keys, then dispatch on the ones we
	// model (modified arrows) and report the rest as unknown.
	if b2 >= '0' && b2 <= '9' {
		params := string(b2)
		var finalByte byte
		for {
			p, ok := next()
			if !ok {
				break
			}
			if (p >= '0' && p <= '9') || p == ';' {
				params += string(p)
				continue
			}
			finalByte = p // the letter/tilde that terminates the CSI
			break
		}
		// "1;5C"/"1;5D" — Ctrl-modified Right/Left. The modifier param is 5
		// (Ctrl); treat any non-1 modifier the same (Alt/Shift word-jump is fine).
		if params == "1;5" || params == "1;3" || params == "1;2" {
			if finalByte == 'C' {
				return Key{Type: KeyCtrlRight}
			}
			if finalByte == 'D' {
				return Key{Type: KeyCtrlLeft}
			}
		}
		// Home/End also arrive as "1~"/"7~" and "4~"/"8~" on some terminals.
		if finalByte == '~' {
			if params == "1" || params == "7" {
				return Key{Type: KeyHome}
			}
			if params == "4" || params == "8" {
				return Key{Type: KeyEnd}
			}
		}
	}
	return Key{Type: KeyUnknown}
}

// RawMode is an RAII-style guard: EnterRawMode puts the controlling terminal
// into raw mode (no canonical line buffering, no echo, no signal generation);
// Close restores the prior mode and releases the handle. Always pair with
// `defer rm.Close()`. The zero value is unusable; obtain one from EnterRawMode.
//
// It reads from the controlling terminal directly (POSIX /dev/tty, Windows
// CONIN$) rather than os.Stdin: that survives a stdin redirected by a
// double-click launcher and a child process (sudo) that reads the tty.
//
// Ctrl-C is delivered as a key (KeyCtrlC), not a signal: raw mode disables the
// terminal's signal generation, so the caller handles cancellation as a normal
// event.
type RawMode struct {
	impl rawModeImpl
}

// EnterRawMode enters raw mode on the controlling terminal. Returns ErrNotATTY
// when there's no terminal (caller falls back to a linear flow) or
// ErrRawModeFailed when the OS refuses raw mode.
func EnterRawMode() (*RawMode, error) {
	impl, err := enterRawMode()
	if err != nil {
		return nil, err
	}
	return &RawMode{impl: impl}, nil
}

// ReadKey blocks until one logical key is available and returns it. It
// propagates a read failure rather than swallowing it (AGENTS: never silently
// swallow errors).
func (r *RawMode) ReadKey() (Key, error) {
	if r == nil {
		return Key{}, fmt.Errorf("tui: ReadKey on nil RawMode: %w", ErrReadFailed)
	}
	return r.impl.readKey()
}

// DiscardPending drops any input already buffered on the terminal without
// blocking. Call before a destructive confirm so a keystroke typed earlier
// (e.g. the Enter that submitted a sudo password) can't auto-select a button
// before the operator sees the screen.
func (r *RawMode) DiscardPending() {
	if r != nil {
		r.impl.discardPending()
	}
}

// Close restores the terminal's prior mode and releases the handle. Idempotent.
func (r *RawMode) Close() {
	if r != nil {
		r.impl.close()
	}
}
