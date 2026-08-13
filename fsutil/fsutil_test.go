package fsutil

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	t.Run("writes a new file with the given perm", func(t *testing.T) {
		require.NoError(t, WriteFileAtomic(path, []byte("hello"), 0o600))
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "hello", string(got))
	})

	t.Run("overwrites and leaves no temp files behind", func(t *testing.T) {
		require.NoError(t, WriteFileAtomic(path, []byte("world"), 0o600))
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "world", string(got))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1) // only the target, no leftover *.tmp.
	})

	t.Run("fails when the parent directory is missing", func(t *testing.T) {
		err := WriteFileAtomic(filepath.Join(dir, "missing", "x"), []byte("x"), 0o600)
		require.Error(t, err)
	})
}

func TestWriteAtomic(t *testing.T) {
	t.Run("streams src and reports the byte count", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bin")
		n, err := WriteAtomic(path, strings.NewReader("payload"), 0o755, nil)
		require.NoError(t, err)
		require.EqualValues(t, 7, n)

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "payload", string(got))

		info, err := os.Stat(path)
		require.NoError(t, err)
		if runtime.GOOS != "windows" { // perms are ACL-governed there
			require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		}
	})

	// The point of the rename: replacing a file that another handle still has
	// open leaves that handle reading the OLD bytes rather than a truncated
	// file. That is what makes re-running the installer over a live install safe.
	t.Run("replaces without disturbing an open handle", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bin")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o755))

		open, err := os.Open(path)
		require.NoError(t, err)
		defer open.Close() //nolint:errcheck // read-only probe

		_, err = WriteAtomic(path, strings.NewReader("new"), 0o755, nil)
		require.NoError(t, err)

		held, err := io.ReadAll(open)
		require.NoError(t, err)
		require.Equal(t, "old", string(held), "the open handle keeps the replaced inode")

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "new", string(got))
	})

	t.Run("a failing verify aborts before the rename", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bin")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o755))

		wantErr := errors.New("checksum mismatch")
		_, err := WriteAtomic(path, strings.NewReader("new"), 0o755, func(int64) error { return wantErr })
		require.ErrorIs(t, err, wantErr)

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "old", string(got), "destination untouched")

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "temp file cleaned up")
	})

	t.Run("verify sees the written byte count", func(t *testing.T) {
		var got int64
		path := filepath.Join(t.TempDir(), "bin")
		_, err := WriteAtomic(path, strings.NewReader("12345"), 0o644, func(n int64) error {
			got = n
			return nil
		})
		require.NoError(t, err)
		require.EqualValues(t, 5, got)
	})
}
