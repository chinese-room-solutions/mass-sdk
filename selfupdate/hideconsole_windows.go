//go:build windows

package selfupdate

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideConsole keeps a detached child from allocating a console window. The
// installer the daemon spawns is a console app talking to the spawn log, not
// to a screen; without this its window flashes open (and stays open for the
// install) every time a GUI-subsystem app updates itself.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
