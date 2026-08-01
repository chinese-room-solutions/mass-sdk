package install

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// A Stage failure is the last step reported: nothing after it runs, and the
// install aborts with the error.
func TestInstallStageFailureReportsOnlyStage(t *testing.T) {
	pointConfigDirAtTemp(t)
	// testApp's exe leaf isn't beside the test binary, so staging finds nothing to
	// copy and fails on the missing staged binary.
	a := testApp()

	var got []Step
	_, err := a.Install(Plan{
		InstallDir: filepath.Join(t.TempDir(), "app"),
		PerUser:    true,
		Hooks:      Hooks{Step: func(s Step, _ error) { got = append(got, s) }},
	})
	require.ErrorIs(t, err, ErrStage)
	require.Equal(t, []Step{StepStage}, got)
}

// pointConfigDirAtTemp redirects os.UserConfigDir at a temp dir so record
// operations can't touch the real user config.
func pointConfigDirAtTemp(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", tmp)
	case "darwin":
		t.Setenv("HOME", tmp) // ~/Library/Application Support
	default:
		t.Setenv("XDG_CONFIG_HOME", tmp)
	}
}

// When the uninstaller runs from inside the install dir, the staged removal is
// self-skipped — the record must be KEPT so the orphaned install dir keeps its
// breadcrumb for a later re-run to find and remove.
func TestUninstallSelfSkippedKeepsRecord(t *testing.T) {
	pointConfigDirAtTemp(t)
	a := testApp()

	ownDir, err := CurrentExecutableDir()
	require.NoError(t, err)
	require.NoError(t, a.SaveRecord(Record{InstallDir: ownDir, DataDir: t.TempDir()}))

	selfSkipped, err := a.Uninstall(ownDir, true)
	require.NoError(t, err)
	require.True(t, selfSkipped, "uninstalling the dir we run from must self-skip")

	rec, err := a.LoadRecord()
	require.NoError(t, err)
	require.NotNil(t, rec, "self-skipped uninstall must keep the install record")
	require.Equal(t, ownDir, rec.InstallDir)
}

// A normal uninstall (install dir elsewhere) removes the record.
func TestUninstallRemovesRecord(t *testing.T) {
	pointConfigDirAtTemp(t)
	a := testApp()

	dir := t.TempDir()
	require.NoError(t, a.SaveRecord(Record{InstallDir: dir, DataDir: t.TempDir()}))

	selfSkipped, err := a.Uninstall(dir, true)
	require.NoError(t, err)
	require.False(t, selfSkipped)

	rec, err := a.LoadRecord()
	require.NoError(t, err)
	require.Nil(t, rec, "a completed uninstall removes the install record")
}
