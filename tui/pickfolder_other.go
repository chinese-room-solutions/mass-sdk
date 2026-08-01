//go:build !windows && !darwin && !linux

package tui

// pickFolder has no native dialog on these platforms, so the form falls back to
// inline path editing (ErrNoPicker).
func pickFolder(string) (string, bool, error) { return "", false, ErrNoPicker }
