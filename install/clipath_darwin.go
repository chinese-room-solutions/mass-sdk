//go:build darwin

package install

import (
	"os"
	"path/filepath"
)

// cliBinDir picks the bin directory a symlink is placed in for the given scope,
// and whether it's already on PATH.
//
//   - system, or user when /usr/local/bin is writable/elevated: /usr/local/bin —
//     the standard Homebrew/manual bin dir, on PATH by default. macOS ships no
//     ~/.local/bin on PATH, so /usr/local/bin is preferred whenever we can write
//     it (a foreign-file check still applies before we clobber anything).
//   - user fallback (can't write /usr/local/bin): ~/.local/bin, with onPath=false
//     so the caller prints a PATH hint.
func (a AppSpec) cliBinDir(scope Scope) (dir string, onPath bool) {
	if scope == ScopeSystem || IsElevated() || CanWriteDir("/usr/local/bin") {
		return "/usr/local/bin", true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	// ~/.local/bin is not on macOS's default PATH — always hint.
	return filepath.Join(home, ".local", "bin"), false
}
