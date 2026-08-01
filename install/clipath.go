package install

import (
	"path/filepath"
	"strings"
)

// CLI-on-PATH exposure makes the installed app binary runnable by its bare name
// from any terminal, the terminal-app analogue of the desktop launcher. Each
// backend is best-effort and idempotent (re-running an install re-establishes
// the same entry); a failure is returned so the setup flow can warn without
// aborting the install.
//
// The mechanism is OS-specific:
//   - Linux/macOS: a symlink to the staged binary in a bin directory on PATH.
//   - Windows: the install directory appended to the user/system PATH env var.
//
// What was created is returned so it can be recorded (Record.CLIPath) and undone
// exactly on uninstall.

// CLIResult reports the outcome of exposing the binary on PATH, for the setup
// flow to show and to record for uninstall.
type CLIResult struct {
	// Created is what was made, to record for an exact uninstall: the symlink path
	// (Linux/macOS) or the install dir added to PATH (Windows). Empty when nothing
	// was created (a foreign file blocked the symlink, or the entry already
	// existed on Windows).
	Created string
	// Hint, when non-empty, is a one-line message the setup flow should print at
	// the end of install: the bin dir isn't on PATH yet (Linux/macOS fallback), or
	// the new shell must be reopened (Windows). Empty when the binary is already
	// reachable.
	Hint string
	// OnPath reports whether the app is reachable by name after this step (the bin
	// dir is on PATH / the PATH var was updated). False when a foreign file blocked
	// the symlink. Drives the install summary's "Run `x` from any terminal" line.
	OnPath bool
}

// LinkOnPath exposes the staged binary on PATH for the given scope. stagedExe is
// the absolute path returned by Stage. Best-effort and idempotent.
func (a AppSpec) LinkOnPath(stagedExe string, scope Scope) (CLIResult, error) {
	return a.linkOnPath(stagedExe, scope)
}

// UnlinkFromPath undoes LinkOnPath using what was recorded in Record.CLIPath
// (the symlink path, or the Windows PATH entry). installDir is the app's install
// directory, so a symlink is removed only when it still points inside it. An
// empty created is a no-op. Removes only what we created; the Windows PATH entry
// is removed by exact match (installDir is unused there).
func (a AppSpec) UnlinkFromPath(created, installDir string, scope Scope) error {
	return a.unlinkFromPath(created, installDir, scope)
}

// installDirOf is the install directory a staged exe belongs to. For a plain
// staging that's the exe's directory; for a macOS .app bundle the staged exe
// sits at <App>.app/Contents/MacOS/<exe>, so the install dir is the .app root
// (the nearest ".app" ancestor).
func installDirOf(stagedExe string) string {
	dir := filepath.Dir(stagedExe)
	for {
		if filepath.Ext(dir) == ".app" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(stagedExe)
		}
		dir = parent
	}
}

// --- Windows PATH-string logic (pure, so it's testable off Windows) ----------

// pathListSep is the separator between entries in a Windows PATH value.
const pathListSep = ";"

// pathContains reports whether pathVal (a ';'-separated PATH string) already
// contains dir, comparing case-insensitively and ignoring a trailing separator
// and surrounding whitespace on each entry (Windows paths are case-insensitive,
// and a stray trailing ';' is common).
func pathContains(pathVal, dir string) bool {
	want := strings.TrimRight(strings.TrimSpace(dir), `\`)
	for _, e := range strings.Split(pathVal, pathListSep) {
		got := strings.TrimRight(strings.TrimSpace(e), `\`)
		if got != "" && strings.EqualFold(got, want) {
			return true
		}
	}
	return false
}

// pathAppend returns pathVal with dir appended, or pathVal unchanged if dir is
// already present. It preserves the existing value verbatim (so REG_EXPAND_SZ
// %vars% survive) and joins with a single ';', avoiding a doubled or leading
// separator when the value is empty or already ends in one.
func pathAppend(pathVal, dir string) string {
	if pathContains(pathVal, dir) {
		return pathVal
	}
	if pathVal == "" {
		return dir
	}
	if strings.HasSuffix(pathVal, pathListSep) {
		return pathVal + dir
	}
	return pathVal + pathListSep + dir
}

// pathRemove returns pathVal with every entry equal to dir removed (case- and
// trailing-slash-insensitively), preserving the order and exact text of the
// remaining entries. Empty segments (from a doubled ';') are dropped so removal
// doesn't leave a stray separator behind.
func pathRemove(pathVal, dir string) string {
	want := strings.TrimRight(strings.TrimSpace(dir), `\`)
	parts := strings.Split(pathVal, pathListSep)
	kept := make([]string, 0, len(parts))
	for _, e := range parts {
		trimmed := strings.TrimRight(strings.TrimSpace(e), `\`)
		if trimmed == "" || strings.EqualFold(trimmed, want) {
			continue
		}
		kept = append(kept, e)
	}
	return strings.Join(kept, pathListSep)
}
