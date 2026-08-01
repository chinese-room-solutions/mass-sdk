//go:build linux || darwin

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// stagedBinary lays down a fake install dir with a runnable "binary" and returns
// its staged-exe path. installDir is <home>/install so it sits under home and
// counts as a user-scoped install we own.
func stagedBinary(t *testing.T, home string) (installDir, stagedExe string) {
	t.Helper()
	installDir = filepath.Join(home, "install")
	require.NoError(t, os.MkdirAll(installDir, 0o755))
	stagedExe = filepath.Join(installDir, "testapp")
	require.NoError(t, os.WriteFile(stagedExe, []byte("#!/bin/sh\n"), 0o755))
	return installDir, stagedExe
}

// userBinDir is where a user-scope symlink lands: ~/.local/bin.
func userBinDir(home string) string { return filepath.Join(home, ".local", "bin") }

func TestLinkOnPathCreatesSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	a := testApp()
	installDir, stagedExe := stagedBinary(t, home)

	res, err := a.LinkOnPath(stagedExe, ScopeUser)
	require.NoError(t, err)

	link := filepath.Join(userBinDir(home), a.ExeName)
	require.Equal(t, link, res.Created)

	// The link exists and resolves to the staged binary.
	target, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, stagedExe, target)

	// It reports ownership of the install dir.
	require.True(t, symlinkOwnedBy(link, installDir))
}

func TestLinkOnPathReplacesOwnedSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	a := testApp()
	installDir, stagedExe := stagedBinary(t, home)

	// A stale link we own (points at an old path inside the install dir).
	binDir := userBinDir(home)
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	link := filepath.Join(binDir, a.ExeName)
	stale := filepath.Join(installDir, "old-binary")
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o755))
	require.NoError(t, os.Symlink(stale, link))

	res, err := a.LinkOnPath(stagedExe, ScopeUser)
	require.NoError(t, err)
	require.Equal(t, link, res.Created)

	// It now points at the freshly-staged exe.
	target, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, stagedExe, target)
}

func TestLinkOnPathSkipsForeignRegularFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	a := testApp()
	_, stagedExe := stagedBinary(t, home)

	// A real file the operator put there (e.g. their own script) at our link name.
	binDir := userBinDir(home)
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	link := filepath.Join(binDir, a.ExeName)
	require.NoError(t, os.WriteFile(link, []byte("do not clobber"), 0o755))

	res, err := a.LinkOnPath(stagedExe, ScopeUser)
	require.NoError(t, err) // best-effort: skipping is not an error
	require.Empty(t, res.Created, "must not claim to have created a link")
	require.False(t, res.OnPath)
	require.NotEmpty(t, res.Hint, "must warn that the file was left in place")

	// The foreign file survives unchanged.
	body, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "do not clobber", string(body))
}

func TestLinkOnPathSkipsForeignSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	a := testApp()
	_, stagedExe := stagedBinary(t, home)

	// A symlink pointing OUTSIDE our install dir — not ours to replace.
	binDir := userBinDir(home)
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	link := filepath.Join(binDir, a.ExeName)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	require.NoError(t, os.WriteFile(elsewhere, []byte("x"), 0o755))
	require.NoError(t, os.Symlink(elsewhere, link))

	res, err := a.LinkOnPath(stagedExe, ScopeUser)
	require.NoError(t, err)
	require.Empty(t, res.Created)
	require.False(t, res.OnPath)

	// Still points where it did.
	target, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, elsewhere, target)
}

func TestUnlinkFromPathRemovesOwnedSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	a := testApp()
	installDir, stagedExe := stagedBinary(t, home)

	res, err := a.LinkOnPath(stagedExe, ScopeUser)
	require.NoError(t, err)
	require.NotEmpty(t, res.Created)

	require.NoError(t, a.UnlinkFromPath(res.Created, installDir, ScopeUser))
	_, err = os.Lstat(res.Created)
	require.True(t, os.IsNotExist(err), "the owned symlink must be gone")
}

func TestUnlinkFromPathPreservesForeignFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	a := testApp()
	installDir, _ := stagedBinary(t, home)

	// A decoy regular file sits at the link path (not created by us). Uninstall is
	// told (via the record) that CLIPath is this path, but must NOT delete it
	// because it isn't a symlink into our install dir.
	binDir := userBinDir(home)
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	decoy := filepath.Join(binDir, a.ExeName)
	require.NoError(t, os.WriteFile(decoy, []byte("keep me"), 0o644))

	require.NoError(t, a.UnlinkFromPath(decoy, installDir, ScopeUser))

	body, err := os.ReadFile(decoy)
	require.NoError(t, err)
	require.Equal(t, "keep me", string(body), "a foreign file at the link path must survive")
}

func TestUnlinkFromPathPreservesForeignSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	a := testApp()
	installDir, _ := stagedBinary(t, home)

	// A symlink pointing outside our install dir, recorded as CLIPath by mistake:
	// unlink must leave it because it isn't owned by us.
	binDir := userBinDir(home)
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	link := filepath.Join(binDir, a.ExeName)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	require.NoError(t, os.WriteFile(elsewhere, []byte("x"), 0o755))
	require.NoError(t, os.Symlink(elsewhere, link))

	require.NoError(t, a.UnlinkFromPath(link, installDir, ScopeUser))
	target, err := os.Readlink(link)
	require.NoError(t, err, "the foreign symlink must survive")
	require.Equal(t, elsewhere, target)
}

func TestUnlinkFromPathEmptyIsNoop(t *testing.T) {
	a := testApp()
	require.NoError(t, a.UnlinkFromPath("", "/opt/testapp", ScopeUser))
}

func TestUnlinkFromPathMissingIsNoop(t *testing.T) {
	home := t.TempDir()
	a := testApp()
	require.NoError(t, a.UnlinkFromPath(filepath.Join(home, "gone"), filepath.Join(home, "install"), ScopeUser))
}
