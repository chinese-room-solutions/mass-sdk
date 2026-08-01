//go:build !windows

package install

import "os"

// isElevated reports whether the process runs as root (uid 0) — the POSIX
// equivalent of an elevated token. Writing to machine-wide locations
// (/opt, /Applications, /usr/local) needs this.
func isElevated() bool { return os.Geteuid() == 0 }
