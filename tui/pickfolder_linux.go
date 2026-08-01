//go:build linux

package tui

import (
	"fmt"
	"os/exec"
	"strings"
)

// pickFolder shells out to a desktop folder chooser if one is on PATH — zenity
// (GTK) first, then kdialog (KDE) — so it works without CGO or a running GTK
// loop (unlike the webview package's GtkFileChooser). On a headless box or a
// minimal desktop where neither is installed it returns ("", false, nil): no
// picker, so the caller edits the path inline. A user cancel is also ok=false.
func pickFolder(title string) (string, bool, error) {
	if path, err := exec.LookPath("zenity"); err == nil {
		return runPicker(path, "--file-selection", "--directory", "--title="+title)
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return runPicker(path, "--getexistingdirectory", ".", "--title", title)
	}
	return "", false, ErrNoPicker // no desktop picker available → edit inline
}

// runPicker invokes a chooser binary and reads the selected path from stdout.
// Both zenity and kdialog exit non-zero on cancel and print nothing, which we
// treat as ok=false (a cancel, not a failure). A non-empty path is success.
func runPicker(name string, args ...string) (string, bool, error) {
	out, err := exec.Command(name, args...).Output()
	path := strings.TrimRight(string(out), "\r\n")
	if err != nil {
		if _, isExit := err.(*exec.ExitError); isExit {
			return "", false, nil // cancel (or "no selection") — fall back to edit
		}
		return "", false, fmt.Errorf("%s: %w", name, err)
	}
	if path == "" {
		return "", false, nil
	}
	return path, true, nil
}
