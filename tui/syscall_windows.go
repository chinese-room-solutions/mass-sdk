//go:build windows

package tui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Console input bindings not exposed by x/sys/windows. We bind ReadConsoleInputW
// and FlushConsoleInputBuffer directly and decode the INPUT_RECORD union by hand.
var (
	modkernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procReadConsoleInputW        = modkernel32.NewProc("ReadConsoleInputW")
	procFlushConsoleInputBuffer  = modkernel32.NewProc("FlushConsoleInputBuffer")
)

// Event types and key constants (winbase.h / wincon.h).
const (
	keyEvent uint16 = 0x0001

	leftCtrlPressed  uint32 = 0x0008
	rightCtrlPressed uint32 = 0x0004

	vkBack   uint16 = 0x08
	vkTab    uint16 = 0x09
	vkReturn uint16 = 0x0D
	vkEscape uint16 = 0x1B
	vkEnd    uint16 = 0x23
	vkHome   uint16 = 0x24
	vkLeft   uint16 = 0x25
	vkUp     uint16 = 0x26
	vkRight  uint16 = 0x27
	vkDown   uint16 = 0x28
)

// inputRecord mirrors INPUT_RECORD: a 2-byte EventType, 2 bytes of padding to
// align the union, then the union payload. KEY_EVENT_RECORD is the largest case
// we read; the buffer is sized to hold the whole record.
//
//	typedef struct _INPUT_RECORD {
//	  WORD  EventType;
//	  union { KEY_EVENT_RECORD KeyEvent; ... } Event;
//	} INPUT_RECORD;
//
//	typedef struct _KEY_EVENT_RECORD {
//	  BOOL  bKeyDown;            // 4 bytes
//	  WORD  wRepeatCount;        // 2
//	  WORD  wVirtualKeyCode;     // 2
//	  WORD  wVirtualScanCode;    // 2
//	  union { WCHAR; CHAR; } uChar; // 2 (+2 pad)
//	  DWORD dwControlKeyState;   // 4
//	} KEY_EVENT_RECORD;
type inputRecord struct {
	eventType uint16
	_         uint16 // pad: the union is DWORD-aligned
	// Event union, decoded via asKeyEvent for KEY_EVENT_RECORD.
	keyDownLo       uint16
	keyDownHi       uint16
	repeatCount     uint16
	virtualKeyCode  uint16
	virtualScanCode uint16
	unicodeChar     uint16
	controlKeyState uint32
}

// keyEventRecord is the decoded KEY_EVENT_RECORD view.
type keyEventRecord struct {
	keyDown         uint32
	repeatCount     uint16
	virtualKeyCode  uint16
	virtualScanCode uint16
	unicodeChar     uint16
	controlKeyState uint32
}

func (r *inputRecord) asKeyEvent() keyEventRecord {
	return keyEventRecord{
		keyDown:         uint32(r.keyDownLo) | uint32(r.keyDownHi)<<16,
		repeatCount:     r.repeatCount,
		virtualKeyCode:  r.virtualKeyCode,
		virtualScanCode: r.virtualScanCode,
		unicodeChar:     r.unicodeChar,
		controlKeyState: r.controlKeyState,
	}
}

func readConsoleInput(h windows.Handle, rec *inputRecord, n uint32, got *uint32) error {
	r1, _, err := procReadConsoleInputW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(rec)),
		uintptr(n),
		uintptr(unsafe.Pointer(got)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}

func flushConsoleInputBuffer(h windows.Handle) {
	// Best-effort: discarding queued input has no recovery path if it fails.
	_, _, _ = procFlushConsoleInputBuffer.Call(uintptr(h))
}
