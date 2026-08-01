package install

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Scope is where an app is installed: per-user (no elevation) or machine-wide
// (needs admin/root). It selects both the install dir and the system-data dir.
type Scope string

const (
	// ScopeUser installs under the user's home — no elevation, current user only.
	ScopeUser Scope = "user"
	// ScopeSystem installs machine-wide (ProgramFiles / /opt / /Applications) for
	// all users — needs an elevated relaunch.
	ScopeSystem Scope = "system"
)

// AvailableScopes lists the scopes an operator may choose, default first. Both
// are offered on every OS: these are user-launched desktop apps (not system
// services), so a per-user install is valid everywhere — including Windows,
// where it lands under %LOCALAPPDATA%\Programs. User leads because it needs no
// elevation.
func AvailableScopes() []Scope { return []Scope{ScopeUser, ScopeSystem} }

// ParseScope maps a label (case-insensitively, "user"/"system") to a Scope.
// Anything else is an error — silently defaulting would mask an operator typo
// (e.g. a misspelled --scope flag) as a per-user install.
func ParseScope(s string) (Scope, error) {
	switch strings.ToLower(s) {
	case string(ScopeUser):
		return ScopeUser, nil
	case string(ScopeSystem):
		return ScopeSystem, nil
	}
	return "", fmt.Errorf("install: unknown scope %q (want %q or %q)", s, ScopeUser, ScopeSystem)
}

// Label is the human-facing scope name for menus ("User" / "System").
func (s Scope) Label() string {
	if s == ScopeSystem {
		return "System"
	}
	return "User"
}

// ScopeInstallDir is the default install directory for a scope: the per-user dir
// (ScopeUser) or the machine-wide dir (ScopeSystem). Falls back to the
// machine-wide dir when the user dir is unresolvable (no home).
func (a AppSpec) ScopeInstallDir(scope Scope) string {
	if scope == ScopeUser {
		if dir := a.UserInstallDir(); dir != "" {
			return dir
		}
	}
	return a.DefaultInstallDir()
}

// SystemDataDir is the machine-wide data directory for a ScopeSystem install,
// per OS convention:
//
//	Windows: %ProgramData%\<Name>
//	macOS:   /Library/Application Support/<Name>
//	Linux:   /var/lib/<Name>
//
// The per-user data dir is the app's own concern (it owns its config/state
// layout), so the app supplies that; the SDK only owns the system convention.
func (a AppSpec) SystemDataDir() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, a.DisplayName)
	case "darwin":
		return filepath.Join("/Library", "Application Support", a.Name)
	default:
		return filepath.Join("/var", "lib", a.Name)
	}
}
