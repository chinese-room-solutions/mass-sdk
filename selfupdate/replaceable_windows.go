//go:build windows

package selfupdate

import "os"

// replaceable is the Windows probe: an open for write. The installer replaces
// the exe by renaming a fresh file over it, and Windows refuses that rename —
// and any write — while some process still runs that exe, so the write-open
// fails under exactly the condition the caller waits out. Renaming the exe
// aside, as the POSIX probe does, would be wrong here: a running executable
// CAN be renamed, only overwritten is denied, so that probe would report
// replaceable while the rename-over the installer performs is still failing.
// A read-only file also fails the write-open, matching the rename-over it
// would refuse too.
func replaceable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	return f.Close() == nil
}
