package connstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := LoadFrom(filepath.Join(t.TempDir(), "auth.json"))
	require.NoError(t, err)
	return s
}

func TestStore_SetGet(t *testing.T) {
	s := newStore(t)

	require.NoError(t, s.Set("http://localhost:3455", "tokA"))
	require.NoError(t, s.Set("https://Work.Example.COM", "tokB"))

	tok, ok := s.Get("http://localhost:3455")
	require.True(t, ok)
	require.Equal(t, "tokA", tok)

	tok, ok = s.Get("http://localhost:3455/") // trailing slash
	require.True(t, ok)
	require.Equal(t, "tokA", tok)

	tok, ok = s.Get("https://work.example.com")
	require.True(t, ok)
	require.Equal(t, "tokB", tok)

	_, ok = s.Get("http://other:1234")
	require.False(t, ok)
}

func TestStore_Delete(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.Set("http://localhost:3455", "tokA"))
	require.NoError(t, s.Delete("http://localhost:3455"))
	_, ok := s.Get("http://localhost:3455")
	require.False(t, ok)
	require.NoError(t, s.Delete("http://localhost:3455")) // idempotent
}

func TestStore_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")

	s1, err := LoadFrom(path)
	require.NoError(t, err)
	require.NoError(t, s1.Set("http://localhost:3455", "tokA"))

	s2, err := LoadFrom(path)
	require.NoError(t, err)
	tok, ok := s2.Get("http://localhost:3455")
	require.True(t, ok)
	require.Equal(t, "tokA", tok)
}

func TestStore_Conn(t *testing.T) {
	s := newStore(t)

	require.NoError(t, s.SetConn("http://localhost:3455", Conn{Token: "tok", CACert: "/etc/ca.pem"}))
	c, ok := s.GetConn("http://localhost:3455/") // trailing slash normalizes
	require.True(t, ok)
	require.Equal(t, Conn{Token: "tok", CACert: "/etc/ca.pem"}, c)

	_, ok = s.GetConn("http://other:1234")
	require.False(t, ok)
}

func TestStore_SetPreservesCACert(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.SetConn("http://localhost:3455", Conn{Token: "old", CACert: "/etc/ca.pem"}))

	// Set is token-only and must not drop the stored CA cert.
	require.NoError(t, s.Set("http://localhost:3455", "new"))
	c, ok := s.GetConn("http://localhost:3455")
	require.True(t, ok)
	require.Equal(t, Conn{Token: "new", CACert: "/etc/ca.pem"}, c)
}

func TestStore_LoadLegacyTokenFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	// The pre-Conn format: endpoint → bare token string.
	require.NoError(t, os.WriteFile(path, []byte(`{"http://localhost:3455":"legacy"}`), 0o600))

	s, err := LoadFrom(path)
	require.NoError(t, err)
	tok, ok := s.Get("http://localhost:3455")
	require.True(t, ok)
	require.Equal(t, "legacy", tok)

	// The next write rewrites the file in the new Conn shape; reloading reads it.
	require.NoError(t, s.SetConn("http://localhost:3455", Conn{Token: "legacy", CACert: "/ca.pem"}))
	s2, err := LoadFrom(path)
	require.NoError(t, err)
	c, ok := s2.GetConn("http://localhost:3455")
	require.True(t, ok)
	require.Equal(t, Conn{Token: "legacy", CACert: "/ca.pem"}, c)
}

// Load's one-time migration: a missing config-path store seeds from the old
// cache-dir store (and persists), the old file stays put, and once the new
// file exists the old one is never consulted again.
func TestLoadMigratesFromOldCacheLocation(t *testing.T) {
	newPath := filepath.Join(t.TempDir(), "config", "mass", "auth.json")
	oldPath := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(oldPath, []byte(`{"http://localhost:3455":{"token":"tok"}}`), 0o600))

	s, err := loadMigrating(newPath, oldPath)
	require.NoError(t, err)
	tok, ok := s.Get("http://localhost:3455")
	require.True(t, ok)
	require.Equal(t, "tok", tok)

	// The migrated contents were persisted at the new path; the old file remains.
	require.FileExists(t, newPath)
	require.FileExists(t, oldPath)

	// A later change lands in the new file only; a fresh load ignores the old one.
	require.NoError(t, s.Set("http://localhost:3455", "rotated"))
	s2, err := loadMigrating(newPath, oldPath)
	require.NoError(t, err)
	tok, ok = s2.Get("http://localhost:3455")
	require.True(t, ok)
	require.Equal(t, "rotated", tok)
	old, err := os.ReadFile(oldPath)
	require.NoError(t, err)
	require.NotContains(t, string(old), "rotated", "the old cache-dir file must not receive updates")
}

// No old store → a plain empty store at the new path, no error.
func TestLoadMigratingWithoutOldStore(t *testing.T) {
	newPath := filepath.Join(t.TempDir(), "auth.json")
	s, err := loadMigrating(newPath, filepath.Join(t.TempDir(), "absent.json"))
	require.NoError(t, err)
	_, ok := s.Get("http://localhost:3455")
	require.False(t, ok)
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"http://localhost:3455", "http://localhost:3455", true},
		{"http://localhost:3455/", "http://localhost:3455", true},
		{"HTTP://Localhost:3455", "http://localhost:3455", true},
		{"https://example.com/api", "https://example.com", true},
		{"  http://x:1  ", "http://x:1", true},
		{"", "", false},
		{"not a url", "", false},
		{"/no/scheme", "", false},
	}
	for _, tt := range tests {
		got, ok := normalize(tt.in)
		require.Equal(t, tt.wantOK, ok, "input=%q", tt.in)
		if tt.wantOK {
			require.Equal(t, tt.want, got, "input=%q", tt.in)
		}
	}
}
