package selfupdate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/install"
	"github.com/stretchr/testify/require"
)

var testApp = install.AppSpec{Name: "mass", DisplayName: "MASS", ExeName: "mass"}

func TestSetupAsset(t *testing.T) {
	want := "mass-setup_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	require.Equal(t, want, SetupAsset(testApp))
}

func TestSetupArgs(t *testing.T) {
	user := install.AppSpec{Name: "mass"}.UserInstallDir()
	require.Contains(t, SetupArgs(user), string(install.ScopeUser))
	require.Contains(t, SetupArgs("/opt/mass"), string(install.ScopeSystem))
	require.Equal(t, []string{"--install", "--install-dir", "/opt/mass", "--scope", "system", "--relaunch"},
		SetupArgs("/opt/mass"))
}

func TestApplyRefusals(t *testing.T) {
	tests := []struct {
		name   string
		record *install.Record
		err    error
		msg    string
	}{
		{name: "no record", msg: "wasn't installed by the MASS installer"},
		{name: "empty install dir", record: &install.Record{}, msg: "wasn't installed by the MASS installer"},
		{
			name:   "unwritable install dir",
			record: &install.Record{InstallDir: filepath.Join(t.TempDir(), "missing")},
			msg:    "installed system-wide",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := Applier{
				App:        testApp,
				StageDir:   t.TempDir(),
				LoadRecord: func() (*install.Record, error) { return tc.record, tc.err },
			}
			err := a.Apply(t.Context(), "v0.4.2")
			require.Error(t, err)
			require.True(t, IsRefusal(err), "want a Refusal, got %v", err)
			require.Contains(t, err.Error(), tc.msg)
		})
	}
}

// A record-lookup failure is a fault, not the operator's situation.
func TestApplyRecordErrorIsNotARefusal(t *testing.T) {
	a := Applier{
		App:        testApp,
		StageDir:   t.TempDir(),
		LoadRecord: func() (*install.Record, error) { return nil, errors.New("boom") },
	}
	err := a.Apply(t.Context(), "v0.4.2")
	require.Error(t, err)
	require.False(t, IsRefusal(err))
	require.Contains(t, err.Error(), "reading the install record")
}

func TestApplyRunsTheDownloadedSetup(t *testing.T) {
	installDir := t.TempDir()
	stage := t.TempDir()
	// A leftover from a previous update must not survive into this one.
	require.NoError(t, os.MkdirAll(filepath.Join(stage, stageLeaf), 0o700))
	stale := filepath.Join(stage, stageLeaf, "stale")
	require.NoError(t, os.WriteFile(stale, []byte("x"), 0o600))

	var gotArgs []string
	a := Applier{
		App:        testApp,
		BaseURL:    "https://example.invalid/owner/repo",
		StageDir:   stage,
		LoadRecord: func() (*install.Record, error) { return &install.Record{InstallDir: installDir}, nil },
		FetchSetup: func(_ context.Context, baseURL, tag, asset, destDir string) (string, error) {
			require.Equal(t, "https://example.invalid/owner/repo", baseURL)
			require.Equal(t, "v0.4.2", tag)
			require.Equal(t, SetupAsset(testApp), asset)
			require.NoFileExists(t, stale)
			return noopExe(t, destDir), nil
		},
		Stdio: func(cmd *exec.Cmd) { gotArgs = cmd.Args[1:] },
	}

	require.NoError(t, a.Apply(t.Context(), "v0.4.2"))
	require.Equal(t, SetupArgs(installDir), gotArgs)
}

func TestApplyNeedsAStageDir(t *testing.T) {
	a := Applier{
		App:        testApp,
		LoadRecord: func() (*install.Record, error) { return &install.Record{InstallDir: t.TempDir()}, nil },
	}
	err := a.Apply(t.Context(), "v0.4.2")
	require.ErrorContains(t, err, "no directory is configured")
}

// noopExe writes a runnable do-nothing program into dir and returns its path.
func noopExe(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "setup.bat")
		require.NoError(t, os.WriteFile(path, []byte("@exit /b 0\r\n"), 0o700))
		return path
	}
	path := filepath.Join(dir, "setup")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	return path
}
