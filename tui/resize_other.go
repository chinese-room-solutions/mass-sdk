//go:build !windows

package tui

// syncBufferToWindow is a no-op off Windows: POSIX terminals don't keep a
// separate oversized screen buffer behind the alt screen, so there's no
// scrollbar to clear (and CSI 8t already sizes the window where honoured).
func syncBufferToWindow(rows, cols int) {}
