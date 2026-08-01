package install

import (
	"os"
	"path/filepath"
	"strings"
)

// IsUserScoped reports whether dir lives under the current user's home, in which
// case installing there (and placing its launcher/record) needs no elevation. A
// path outside home — /opt, /usr, /Applications, %ProgramFiles% — is a
// machine-wide install. The per-OS UserInstallDir values all sit under home, so
// the default install is always user-scoped; this classifies an operator's
// custom path. Returns false when home can't be resolved (treat as needing
// elevation, the safe default).
func IsUserScoped(dir string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(home, abs)
	if err != nil {
		return false
	}
	// rel == ".." or starts with "../" means abs escapes home.
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// NeedsElevation reports whether installing to (or removing from) dir requires an
// admin/root relaunch: only when the target is machine-wide AND this process
// isn't already elevated AND it can't write there as-is. This is the single gate
// the setup flow consults so sudo/UAC is requested ONLY when genuinely needed —
// a user-scoped dir, an already-elevated process, or a directory the user
// happens to own (e.g. a chowned /opt/<app>) all skip the prompt.
func NeedsElevation(dir string) bool {
	return !IsUserScoped(dir) && !IsElevated() && !CanWriteDir(dir)
}

// IsElevated reports whether this process can write to machine-wide locations
// without further elevation: an elevated (Administrator) token on Windows, uid 0
// on POSIX. Lets the setup flow detect "needs admin" before attempting a
// privileged install (staging into ProgramFiles) so it can offer a UAC/sudo
// relaunch up front rather than failing mid-way.
func IsElevated() bool { return isElevated() }

// ElevationOutcome is what RelaunchElevated reports. The two success modes are
// deliberately distinct because the platforms elevate differently: Windows
// spawns a separate elevated process (the work has NOT run yet when the call
// returns), POSIX re-runs the binary under sudo synchronously (the work HAS run
// when the call returns). Callers must act on that difference — exit
// immediately vs. render the child's result in this process.
type ElevationOutcome int

const (
	// ElevationFailed: elevation was unavailable or the elevated run failed —
	// sudo missing, ShellExecuteEx error, or (POSIX) the sudo child exited
	// non-zero, which also covers a declined/failed password prompt (sudo's exit
	// code doesn't distinguish the two). The caller stays unelevated and reports
	// the original permission error.
	ElevationFailed ElevationOutcome = iota
	// ElevationDeclined: the operator explicitly refused the elevation prompt.
	// Reported on Windows (UAC dismissed); POSIX can't tell a declined sudo
	// prompt from a failed child and reports ElevationFailed instead.
	ElevationDeclined
	// ElevatedChildStarted: a separate elevated process was launched (Windows
	// UAC). The privileged work has NOT completed yet — the child does it in its
	// own window — so the caller must exit immediately: two copies must not run.
	ElevatedChildStarted
	// ElevatedWorkSucceeded: the elevated re-run completed successfully (POSIX
	// sudo, run synchronously on this terminal). The caller continues in THIS
	// process, e.g. to render the result screen and keep the window open.
	ElevatedWorkSucceeded
)

// String returns a stable lower-case name, for logs and status lines.
func (o ElevationOutcome) String() string {
	switch o {
	case ElevationDeclined:
		return "elevation declined"
	case ElevatedChildStarted:
		return "elevated child started"
	case ElevatedWorkSucceeded:
		return "elevated work succeeded"
	default:
		return "elevation failed"
	}
}

// RelaunchElevated re-runs this executable with administrator/root rights,
// passing args verbatim so the elevated copy performs the chosen action
// non-interactively.
//
// On Windows it triggers a UAC prompt and spawns a separate elevated process:
// ElevatedChildStarted once that process is launched (the caller must exit
// immediately — the work happens in the child), ElevationDeclined when the
// operator dismisses UAC, ElevationFailed on any other error.
//
// On POSIX it re-runs under sudo synchronously on the same terminal, blocking
// until the elevated copy finishes: ElevatedWorkSucceeded when that copy exited
// 0 (the caller then renders the result in THIS process, keeping the window
// open), ElevationFailed when sudo is unavailable, couldn't start, or the child
// exited non-zero (including a declined password prompt).
func RelaunchElevated(args []string) ElevationOutcome { return relaunchElevated(args) }

// CanWriteDir reports whether the current process can create files in dir
// (creating dir if absent). Used to decide whether an install location needs
// elevation: a user-scoped dir is writable as-is, a machine-wide one (ProgramFiles
// /opt) typically is not. Cleans up any directory/probe it created.
//
// This is a positive capability probe rather than inferring from IsElevated,
// because the answer depends on the specific directory's ACLs, not just the
// token (e.g. an admin-writable subdir, or a user who owns /opt/<app>).
func CanWriteDir(dir string) bool {
	// Walk up to the nearest existing ancestor; that's the dir whose write
	// permission actually governs whether we can create the target.
	probe := dir
	for {
		if _, err := os.Stat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break // reached the root without finding an existing dir
		}
		probe = parent
	}
	f, err := os.CreateTemp(probe, ".write-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}
