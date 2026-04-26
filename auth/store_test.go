package auth

import (
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
