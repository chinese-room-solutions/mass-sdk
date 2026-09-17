//go:build !windows

package selfupdate

import "os/exec"

// hideConsole is a Windows-only concern; there is no console to hide.
func hideConsole(*exec.Cmd) {}
