package fsutil

import (
	"os"
	"path/filepath"
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
