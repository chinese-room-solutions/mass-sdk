package selfupdate

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chinese-room-solutions/mass-sdk/install"
)

// The --relaunch half of the self-update, run inside the app's setup binary:
// the app asked this installer to stage a newer build over its own install and
// then exited. Two things follow — the staged exe may still be locked while the
// old processes wind down (Windows holds a running image open), and once the new
// one is in place somebody has to start it again, since no operator is sitting
// at this terminal.

const (
	// ReplaceableWait bounds the wait for the old build's processes to exit. Past
	// it the install proceeds anyway: a stage that fails reports its own error,
	// which is a better message than a silent give-up here.
	ReplaceableWait = 30 * time.Second
	// replaceablePoll is how often the exe is probed meanwhile.
	replaceablePoll = 200 * time.Millisecond
)

// WaitReplaceable blocks until exe can be replaced (POSIX: at once; Windows:
// once every process running it has exited), or until timeout. A path that
// doesn't exist yet is replaceable — a first install has nothing to wait for.
func WaitReplaceable(exe string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for !Replaceable(exe) {
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(replaceablePoll)
	}
}

// StartApp launches the freshly staged app detached, with no arguments — bare
// `<app>` is the desktop app. The installer exits immediately after, so the
// child must not be tied to it.
func StartApp(app install.AppSpec, installDir string) error {
	exe := app.StagedExePath(installDir)
	cmd := exec.Command(exe) //nolint:gosec // the exe we just staged.
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", exe, err)
	}
	return cmd.Process.Release()
}
