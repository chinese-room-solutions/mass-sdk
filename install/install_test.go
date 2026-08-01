package install

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func testApp() AppSpec {
	return AppSpec{Name: "testapp", DisplayName: "TestApp", ExeName: "testapp"}
}

func TestExeLeaf(t *testing.T) {
	a := testApp()
	if runtime.GOOS == "windows" {
		require.Equal(t, "testapp.exe", a.ExeLeaf())
	} else {
		require.Equal(t, "testapp", a.ExeLeaf())
	}
}

func TestDefaultInstallDir(t *testing.T) {
	a := testApp()
	dir := a.DefaultInstallDir()
	require.NotEmpty(t, dir)
	switch runtime.GOOS {
	case "windows":
		require.Contains(t, dir, "TestApp")
	case "darwin":
		require.Equal(t, "/Applications/TestApp.app", dir)
	default:
		require.Equal(t, "/opt/testapp", dir)
	}
}

func TestStagedExePath(t *testing.T) {
	a := testApp()
	got := a.StagedExePath(filepath.FromSlash("/some/dir"))
	require.Equal(t, filepath.Join("/some/dir", a.ExeLeaf()), got)
}

func TestIsSharedLibrary(t *testing.T) {
	tests := map[string]map[string]bool{
		"windows": {"foo.dll": true, "FOO.DLL": true, "foo.so": false, "foo.exe": false},
		"linux":   {"libfoo.so": true, "libfoo.so.1": true, "libfoo.so.1.2": true, "foo.dll": false},
		"darwin":  {"libfoo.dylib": true, "libfoo.so": false, "foo.dll": false},
	}
	for name, want := range tests[runtime.GOOS] {
		require.Equal(t, want, isSharedLibrary(name), "name=%s", name)
	}
}

func TestSameDir(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()

	// A symlink pointing at dir must resolve as the same directory.
	link := filepath.Join(t.TempDir(), "link")
	symlinked := true
	if err := os.Symlink(dir, link); err != nil {
		symlinked = false // Windows without privilege; skip that case
	}

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", dir, dir, true},
		{"distinct", dir, other, false},
		{"dot segment", dir, filepath.Join(dir, "."), true},
		{"trailing separator", dir, dir + string(filepath.Separator), true},
		{"nested is not same", dir, filepath.Join(dir, "sub"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SameDir(tc.a, tc.b)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	if symlinked {
		t.Run("symlink resolves to target", func(t *testing.T) {
			got, err := SameDir(dir, link)
			require.NoError(t, err)
			require.True(t, got)
		})
	}
}

func TestRecordRoundTrip(t *testing.T) {
	// Point UserConfigDir at a temp location via the platform's env var.
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", tmp)
	case "darwin":
		t.Setenv("HOME", tmp) // ~/Library/Application Support
	default:
		t.Setenv("XDG_CONFIG_HOME", tmp)
	}

	a := testApp()

	// Absent initially.
	rec, err := a.LoadRecord()
	require.NoError(t, err)
	require.Nil(t, rec)

	// Save then load.
	want := Record{InstallDir: filepath.FromSlash("/opt/testapp"), DataDir: filepath.FromSlash("/var/testapp")}
	require.NoError(t, a.SaveRecord(want))

	got, err := a.LoadRecord()
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, want, *got)

	// Remove → absent again.
	require.NoError(t, a.RemoveRecord())
	got, err = a.LoadRecord()
	require.NoError(t, err)
	require.Nil(t, got)
	// Removing a missing record is not an error.
	require.NoError(t, a.RemoveRecord())
}

func TestStageCopiesBinaryAndLibs(t *testing.T) {
	a := testApp()
	src := t.TempDir()
	dst := t.TempDir()

	// Lay down the binary, a matching shared lib, and an unrelated file.
	mustWrite(t, filepath.Join(src, a.ExeLeaf()), "binary")
	libName := sampleLibName()
	mustWrite(t, filepath.Join(src, libName), "lib")
	mustWrite(t, filepath.Join(src, "readme.txt"), "ignored")

	// Stage copies from the running binary's dir, so exercise copySiblings
	// directly (the dir-scoped half) rather than os.Executable.
	require.NoError(t, a.copySiblings(src, dst))

	requireFile(t, filepath.Join(dst, a.ExeLeaf()))
	requireFile(t, filepath.Join(dst, libName))
	requireNoFile(t, filepath.Join(dst, "readme.txt"))
}

// A staging failure carries BOTH the ErrStage sentinel and the underlying
// cause in its chain, so callers can errors.Is against either (e.g.
// os.ErrPermission to suggest elevation).
func TestStageErrorKeepsCauseChain(t *testing.T) {
	a := testApp()
	err := a.copySiblings(filepath.Join(t.TempDir(), "does-not-exist"), t.TempDir())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStage)
	require.ErrorIs(t, err, os.ErrNotExist, "the cause must survive the sentinel wrap")
}

func TestRemoveStagedInstallMissingIsOK(t *testing.T) {
	removed, selfSkipped, err := RemoveStagedInstall(filepath.Join(t.TempDir(), "does-not-exist"))
	require.NoError(t, err)
	require.False(t, selfSkipped)
	require.Equal(t, 0, removed)
}

func TestRemoveStagedInstallRemoves(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	mustWrite(t, filepath.Join(sub, "a"), "x")
	mustWrite(t, filepath.Join(sub, "b"), "y")

	removed, selfSkipped, err := RemoveStagedInstall(sub)
	require.NoError(t, err)
	require.False(t, selfSkipped)
	require.Equal(t, 2, removed)
	requireNoFile(t, sub)
}

func TestCanWriteDir(t *testing.T) {
	require.True(t, CanWriteDir(t.TempDir()))
	// A path under a writable temp dir that doesn't exist yet: governed by the
	// nearest existing ancestor, which is writable.
	require.True(t, CanWriteDir(filepath.Join(t.TempDir(), "new", "deep")))

	// A child of a read-only parent isn't creatable. POSIX-only and not as root
	// (root bypasses the mode bits, and Windows ACLs don't map to chmod).
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		ro := t.TempDir()
		require.NoError(t, os.Chmod(ro, 0o500)) // r-x: no write
		t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
		require.False(t, CanWriteDir(filepath.Join(ro, "child")))
	}
}

// --- helpers ----------------------------------------------------------------

func sampleLibName() string {
	switch runtime.GOOS {
	case "windows":
		return "dep.dll"
	case "darwin":
		return "libdep.dylib"
	default:
		return "libdep.so.1"
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.NoError(t, err, "expected file %s", path)
}

func requireNoFile(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "expected %s to be absent", path)
}
