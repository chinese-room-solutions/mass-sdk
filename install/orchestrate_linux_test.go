//go:build linux

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Install reports every step it reaches, exactly once and in order — and a
// best-effort step that fails (the launcher here) neither stops the install nor
// comes back as an error, since the binary is staged and runnable either way.
//
// Linux-only: this drives the REAL launcher and PATH steps, which are only fully
// redirectable into a temp tree here (XDG_DATA_HOME + HOME); on Windows the PATH
// step writes to the registry.
func TestInstallStepReporting(t *testing.T) {
	tests := []struct {
		name string
		// blockLauncher points XDG_DATA_HOME below a regular file, so the launcher's
		// MkdirAll fails with ENOTDIR while every other step still works.
		blockLauncher bool
		wantFailed    []Step
	}{
		{name: "every step succeeds"},
		{name: "the launcher fails", blockLauncher: true, wantFailed: []Step{StepLauncher}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)                                     // ~/.local/bin for the PATH symlink
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config")) // the install record
			dataHome := filepath.Join(home, "share")                   // the .desktop entry
			if tt.blockLauncher {
				blocked := filepath.Join(home, "not-a-dir")
				require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))
				dataHome = filepath.Join(blocked, "share")
			}
			t.Setenv("XDG_DATA_HOME", dataHome)

			a := stageableApp(t)
			installDir := filepath.Join(t.TempDir(), "app")

			var got, gotFailed []Step
			res, err := a.Install(Plan{
				InstallDir: installDir,
				DataDir:    filepath.Join(home, "data"),
				PerUser:    true,
				Hooks: Hooks{Step: func(s Step, err error) {
					got = append(got, s)
					if err != nil {
						gotFailed = append(gotFailed, s)
					}
				}},
			})

			require.NoError(t, err, "a best-effort failure must not fail the install")
			require.Equal(t, []Step{StepStage, StepLauncher, StepPath, StepRecord}, got)
			require.Equal(t, tt.wantFailed, gotFailed)

			// The install ran to the end: the binary is staged and the record written.
			require.FileExists(t, res.StagedExe)
			rec, rerr := a.LoadRecord()
			require.NoError(t, rerr)
			require.NotNil(t, rec)
			require.Equal(t, installDir, rec.InstallDir)

			if tt.blockLauncher {
				require.Empty(t, res.LauncherPath, "a failed launcher reports no path")
			} else {
				require.FileExists(t, res.LauncherPath)
			}
		})
	}
}

// stageableApp is an AppSpec whose app binary IS the running test binary, so
// Stage's copy-siblings mode finds it beside itself and staging succeeds without
// planting a fake binary in the build directory.
func stageableApp(t *testing.T) AppSpec {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return AppSpec{Name: "testapp", DisplayName: "TestApp", ExeName: filepath.Base(exe)}
}
