//go:build windows

package tui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Console window/buffer sizing not exposed by x/sys/windows. CSI 8t resizes the
// WINDOW, but conhost/Windows Terminal keep a tall screen BUFFER — so a freshly
// launched console shows a scrollbar over the empty buffer rows below the form
// even though the visible content fits. Matching the buffer height to the window
// removes it. We bind SetConsoleScreenBufferSize + SetConsoleWindowInfo directly.
var (
	procSetConsoleScreenBufferSize = modkernel32.NewProc("SetConsoleScreenBufferSize")
	procSetConsoleWindowInfo       = modkernel32.NewProc("SetConsoleWindowInfo")
)

// syncBufferToWindow shrinks the console screen buffer so its height matches the
// window, eliminating the scrollbar a short form leaves in a fresh console. rows
// and cols are the target window size (just requested via CSI 8t); we use them
// rather than re-querying because that resize is async and a query here can still
// report the pre-resize size. Best-effort: any failure (redirected handle,
// restrictive host) leaves the console as-is. The window must fit inside the
// buffer, so we anchor the window at the buffer top-left, then size the buffer.
func syncBufferToWindow(rows, cols int) {
	if rows <= 0 || cols <= 0 {
		return
	}
	fd := windows.Handle(windows.Stdout)
	w, h := int16(cols), int16(rows)

	// 1. Anchor the visible window at the buffer's top-left so shrinking the
	//    buffer underneath it stays valid (window must fit inside the buffer).
	top := windows.SmallRect{Left: 0, Top: 0, Right: w - 1, Bottom: h - 1}
	setConsoleWindowInfo(fd, &top)

	// 2. Shrink the buffer to exactly the window size → no scrollback, no
	//    scrollbar.
	setConsoleScreenBufferSize(fd, windows.Coord{X: w, Y: h})

	// 3. Re-assert the window rect in case the buffer change nudged it.
	setConsoleWindowInfo(fd, &top)
}

func setConsoleScreenBufferSize(fd windows.Handle, size windows.Coord) {
	// SetConsoleScreenBufferSize takes the COORD by value, packed into a DWORD.
	packed := uintptr(uint32(uint16(size.X)) | uint32(uint16(size.Y))<<16)
	_, _, _ = procSetConsoleScreenBufferSize.Call(uintptr(fd), packed)
}

func setConsoleWindowInfo(fd windows.Handle, rect *windows.SmallRect) {
	// bAbsolute = TRUE: the rect is absolute screen-buffer coordinates.
	_, _, _ = procSetConsoleWindowInfo.Call(uintptr(fd), 1, uintptr(unsafe.Pointer(rect)))
}
