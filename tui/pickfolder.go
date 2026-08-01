package tui

import "errors"

// ErrNoPicker means the platform has no usable folder dialog (e.g. a headless
// Linux box with neither zenity nor kdialog). The form treats it as the signal
// to fall back to inline path editing — distinct from a user cancel, which
// leaves the field unchanged and does NOT open the editor.
var ErrNoPicker = errors.New("tui: no native folder picker available")

// PickFolder opens the OS's native folder-selection dialog and returns the
// chosen absolute path. It is windowless and CGO-free: the installer is a
// console app with no GUI event loop, so this can't reuse the webview package's
// window-parented picker.
//
//   - Windows: the pure-Go IFileOpenDialog (syscall, no parent window).
//   - macOS:   `osascript` "choose folder" (always present, no extra deps).
//   - Linux:   `zenity`/`kdialog` if on PATH, else ErrNoPicker.
//
// Outcomes:
//   - chosen:        (path, true, nil)
//   - user cancel:   ("", false, nil)        — leave the field as-is
//   - no picker:     ("", false, ErrNoPicker) — caller edits inline
//   - picker failed: ("", false, err)        — surface the message
func PickFolder(title string) (path string, ok bool, err error) {
	return pickFolder(title)
}
