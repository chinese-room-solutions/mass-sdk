//go:build linux

package install

import (
	"os"
	"path/filepath"
)

// cliBinDir is the bin directory a symlink is placed in for the given scope, and
// whether that directory is already on PATH (so the caller knows to emit a hint).
//
//   - user:   ~/.local/bin — on PATH by the systemd/profile convention. Returns
//     onPath=false when it doesn't yet exist or PATH doesn't list it, so the
//     caller prints a one-line hint.
//   - system: /usr/local/bin — a standard PATH member on every distro.
func (a AppSpec) cliBinDir(scope Scope) (dir string, onPath bool) {
	if scope == ScopeSystem {
		return "/usr/local/bin", true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	dir = filepath.Join(home, ".local", "bin")
	return dir, dirOnPath(dir)
}

// dirOnPath reports whether dir is listed in the process's PATH and exists — the
// two conditions under which a symlink there is immediately runnable by name.
func dirOnPath(dir string) bool {
	if _, err := os.Stat(dir); err != nil {
		return false
	}
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == "" {
			continue
		}
		if same, _ := SameDir(p, dir); same {
			return true
		}
	}
	return false
}
