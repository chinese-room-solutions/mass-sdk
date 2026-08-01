//go:build darwin

package tui

import (
	"fmt"
	"os/exec"
	"strings"
)

// pickFolder shells out to osascript's "choose folder", which is part of every
// macOS install — no CGO and no running Cocoa loop required (unlike the
// webview package's NSOpenPanel). The script prints the POSIX path; a user
// cancel makes osascript exit non-zero with "User canceled", which we map to
// ok=false rather than an error.
func pickFolder(title string) (string, bool, error) {
	prompt := title
	if prompt == "" {
		prompt = "Select a folder"
	}
	// `choose folder` returns an alias; `POSIX path of` converts it to a path.
	script := fmt.Sprintf(
		`POSIX path of (choose folder with prompt %q)`, prompt)

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		if ee, isExit := err.(*exec.ExitError); isExit &&
			strings.Contains(string(ee.Stderr), "User canceled") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("osascript choose folder: %w", err)
	}
	return strings.TrimRight(string(out), "\r\n"), true, nil
}
