//go:build !windows

package install

import (
	"os"
	"os/exec"
	"os/signal"
)

// relaunchElevated re-runs this process under sudo, synchronously (sudo prompts
// for the password inline on the same terminal, unlike Windows' separate elevated
// window). It blocks until the elevated copy finishes and returns
// ElevatedWorkSucceeded when that copy exited 0 — the caller then draws the
// result screen in THIS process and keeps the window open (an AppImage runs the
// child inline, so if this process exited the konsole window would close before
// the operator saw anything). Returns ElevationFailed when sudo is unavailable,
// couldn't start, or the child exited non-zero — which also covers a declined
// or failed password prompt, so ElevationDeclined is never reported here.
func relaunchElevated(args []string) ElevationOutcome {
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return ElevationFailed
	}
	// Inside an AppImage, os.Executable() points into the per-process FUSE mount
	// (/tmp/.mount_*), which a fresh sudo process can't see — re-run the stable
	// on-disk path the runtime exposes via $APPIMAGE instead. The AppImage's
	// launcher forwards these args through to the binary. Outside an AppImage,
	// the executable's own path is the thing to re-run.
	exe := os.Getenv("APPIMAGE")
	if exe == "" {
		exe, err = os.Executable()
		if err != nil {
			return ElevationFailed
		}
	}
	cmd := exec.Command(sudo, append([]string{exe}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	// Ctrl-C at sudo's password prompt sends SIGINT to the whole foreground
	// process group — sudo AND this parent. Without this, the default SIGINT kills
	// the setup process, so it can't return to its form (and the bundle's terminal
	// shows a "/bin/sh crashed" warning). Catch SIGINT in the parent and discard
	// it for the duration of the prompt. Use Notify (not Ignore): a caught signal
	// is reset to its default in the exec'd child, so sudo still sees Ctrl-C, exits
	// non-zero, and we report that as a failed elevation for the caller to handle
	// (re-show the form / Back-Exit). An ignored signal would instead be inherited,
	// preventing the operator from cancelling the prompt at all.
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	defer signal.Stop(sigint)

	// The elevated child did the privileged work (and its own non-interactive
	// output); success/failure is its exit code. Returning (not os.Exit) lets the
	// caller render the result + Back/Exit and keep the window open.
	if cmd.Run() != nil {
		return ElevationFailed
	}
	return ElevatedWorkSucceeded
}
