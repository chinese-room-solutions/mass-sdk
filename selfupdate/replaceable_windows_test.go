//go:build windows

package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReplaceableRunningImage pins the Windows contract the update wait is
// built on: while a process runs the exe, Replaceable must report false — even
// though renaming that exe aside succeeds — or the installer races the app's
// exit and its rename-over fails with Access denied.
func TestReplaceableRunningImage(t *testing.T) {
	src, err := os.Executable()
	require.NoError(t, err)
	body, err := os.ReadFile(src)
	require.NoError(t, err)
	exe := filepath.Join(t.TempDir(), "app.exe")
	require.NoError(t, os.WriteFile(exe, body, 0o755))

	cmd := exec.Command(exe, "-test.run="+t.Name()+"Helper", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	require.Eventually(t, func() bool { return !Replaceable(exe) },
		5*time.Second, 100*time.Millisecond, "the exe must read as replaceable only once its process is gone")

	_ = cmd.Process.Kill()
	_ = cmd.Wait() // a killed process reports a non-zero exit; that is the point
	require.True(t, Replaceable(exe), "with no process left, the exe must read as replaceable")
}

// TestReplaceableRunningImageHelper is the sleeper the copied test binary
// runs; it never runs in the parent (GO_WANT_HELPER_PROCESS gates it).
func TestReplaceableRunningImageHelper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process only")
	}
	time.Sleep(30 * time.Second)
}
