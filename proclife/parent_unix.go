//go:build !windows

package proclife

import "os"

// parentLost compares against the pid seen at the call, rather than testing for
// pid 1: an orphan is reparented to init only where nothing else has claimed the
// role, and under a subreaper (systemd's user manager, a container's pid 1) it
// lands on that instead.
func parentLost() func() bool {
	start := os.Getppid()
	return func() bool { return os.Getppid() != start }
}
