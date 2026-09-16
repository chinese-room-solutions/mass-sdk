//go:build !windows

package selfupdate

import "os"

// replaceable is the POSIX probe: the rename the installer will perform, done
// harmlessly — path is renamed aside and back. A rename over a running binary
// always succeeds on POSIX, so this is normally true; it fails only when the
// directory forbids renames. If putting the file back fails it now lives under
// the probe name — report false so the caller keeps waiting rather than acting
// on a half-moved install.
func replaceable(path string) bool {
	probe := path + ".replaceprobe"
	if err := os.Rename(path, probe); err != nil {
		return false
	}
	return os.Rename(probe, path) == nil
}
