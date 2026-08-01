package install

// Launchers are the desktop-app equivalent of "register a service": they make
// the installed GUI launchable the way the OS expects — a Start Menu shortcut on
// Windows, a .desktop entry on Linux, the .app placement on macOS. Each backend
// is best-effort and idempotent (re-running an install refreshes the launcher);
// a failure is returned so the setup flow can warn without aborting the install.

// LauncherSpec describes where a launcher should point.
type LauncherSpec struct {
	// ExePath is the absolute path of the staged app binary the launcher runs.
	ExePath string
	// IconPath is an optional absolute path to an icon (.ico on Windows, a PNG
	// on Linux); empty uses the binary's embedded icon / a default.
	IconPath string
	// PerUser installs the launcher for the current user only (no elevation)
	// rather than machine-wide (all users). Mirrors the install-dir choice.
	PerUser bool
}

// CreateLauncher creates/refreshes the OS launcher for the app. Best-effort and
// idempotent. Returns the path of the launcher created (for logging/uninstall)
// and any error.
func (a AppSpec) CreateLauncher(spec LauncherSpec) (string, error) {
	return a.createLauncher(spec)
}

// RemoveLauncher removes the OS launcher created by CreateLauncher. Missing is
// not an error.
func (a AppSpec) RemoveLauncher(perUser bool) error {
	return a.removeLauncher(perUser)
}
