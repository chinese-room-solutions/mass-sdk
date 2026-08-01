// Package install is the OS-aware mechanism for installing a desktop GUI app
// (MASS, Grimoire) from a terminal setup flow: staging the binary + its sibling
// assets into a per-OS install directory, recording where it went, creating
// launchers (Start Menu / .desktop / app placement), and uninstalling — all of
// it parameterized over an AppSpec so nothing app-specific lives in the package.
//
// These apps are normal user-launched GUI programs, NOT system services: there
// is deliberately no systemd/launchd/SCM registration, no "start at boot", no
// session-0 daemon. That part of the worker's installer (which IS a service)
// does not apply here. What carries over is the service-free machinery —
// staging, the install-record breadcrumb, elevation detection, and per-OS path
// conventions — ported from the worker's service.cpp as the design reference.
package install

import (
	"os"
	"path/filepath"
	"runtime"
)

// AppSpec is the identity of the app being installed — the only app-specific
// input the package takes. Everything else (where ProgramFiles is, how to make a
// shortcut) is OS convention the package owns.
type AppSpec struct {
	// Name is the install/data directory leaf and the stable identifier, e.g.
	// "mass" or "grimoire". Lowercase, no spaces.
	Name string
	// DisplayName is the human-facing label for shortcuts / menu entries, e.g.
	// "MASS" or "Grimoire".
	DisplayName string
	// ExeName is the leaf filename of the app binary WITHOUT the platform suffix,
	// e.g. "mass". ExeLeaf appends ".exe" on Windows.
	ExeName string
	// BundleID is the reverse-DNS identifier baked into the macOS Info.plist
	// (CFBundleIdentifier), e.g. "solutions.chineseroom.mass". Empty falls back
	// to "com.<Name>". Ignored off macOS.
	BundleID string
}

// ExeLeaf is the app binary's filename including the platform's executable
// suffix (".exe" on Windows).
func (a AppSpec) ExeLeaf() string {
	if runtime.GOOS == "windows" {
		return a.ExeName + ".exe"
	}
	return a.ExeName
}

// DefaultInstallDir is the machine-wide location for the app's installed code,
// per OS convention:
//
//	Windows: %ProgramFiles%\<DisplayName>
//	macOS:   /Applications/<DisplayName>.app  (an app bundle dir)
//	Linux:   /opt/<Name>
//
// Writing here generally needs elevation (see IsElevated). The data directory
// (config/state) is separate and user-scoped — the app owns that path.
func (a AppSpec) DefaultInstallDir() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("ProgramFiles")
		if base == "" {
			base = `C:\Program Files`
		}
		return filepath.Join(base, a.DisplayName)
	case "darwin":
		return filepath.Join("/Applications", a.DisplayName+".app")
	default:
		return filepath.Join("/opt", a.Name)
	}
}

// UserInstallDir is the per-user location for the app's installed code, the
// no-elevation alternative to DefaultInstallDir:
//
//	Windows: %LOCALAPPDATA%\Programs\<DisplayName>
//	macOS:   ~/Applications/<DisplayName>.app
//	Linux:   ~/.local/lib/<Name>
//
// On Linux the program files go under ~/.local/lib (the user-local analogue of
// /usr/lib), NOT ~/.local/share — the latter is the XDG *data* root, where an
// app's own user data lives, so reusing it for the install would collide the
// program dir with the data dir. Returns "" if home/local-appdata can't be
// resolved.
func (a AppSpec) UserInstallDir() string {
	switch runtime.GOOS {
	case "windows":
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, "Programs", a.DisplayName)
		}
		return ""
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Applications", a.DisplayName+".app")
		}
		return ""
	default:
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "lib", a.Name)
		}
		return ""
	}
}

// StagedExePath is where the app binary lands inside an install directory. On
// macOS, when installDir is a ".app" bundle (the default there), the binary
// belongs at Contents/MacOS/<exe> per Apple's bundle layout — Finder only treats
// the directory as a launchable app when the executable sits there alongside a
// Contents/Info.plist. Elsewhere (and for a non-.app dir) it sits at the top.
func (a AppSpec) StagedExePath(installDir string) string {
	if runtime.GOOS == "darwin" && filepath.Ext(installDir) == ".app" {
		return filepath.Join(installDir, "Contents", "MacOS", a.ExeLeaf())
	}
	return filepath.Join(installDir, a.ExeLeaf())
}

// CurrentExecutableDir is the directory of the running binary (the setup exe),
// used as the source for sibling assets when staging an unpackaged run.
func CurrentExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// EvalSymlinks so a launcher symlink resolves to the real install tree.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}
