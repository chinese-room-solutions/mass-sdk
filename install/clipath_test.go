package install

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The Windows PATH-string logic is factored pure so it's exercised on any OS
// (these tests run on the Linux CI box).

func TestPathContains(t *testing.T) {
	tests := []struct {
		name    string
		pathVal string
		dir     string
		want    bool
	}{
		{"empty", "", `C:\App`, false},
		{"present exact", `C:\Windows;C:\App`, `C:\App`, true},
		{"case insensitive", `C:\Windows;C:\app`, `C:\APP`, true},
		{"trailing slash on entry", `C:\Windows;C:\App\`, `C:\App`, true},
		{"trailing slash on dir", `C:\Windows;C:\App`, `C:\App\`, true},
		{"whitespace around entry", `C:\Windows; C:\App `, `C:\App`, true},
		{"absent", `C:\Windows;C:\Other`, `C:\App`, false},
		{"substring not a match", `C:\Windows;C:\AppData`, `C:\App`, false},
		{"expand var preserved and matched", `%SystemRoot%;C:\App`, `C:\App`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, pathContains(tc.pathVal, tc.dir))
		})
	}
}

func TestPathAppend(t *testing.T) {
	tests := []struct {
		name    string
		pathVal string
		dir     string
		want    string
	}{
		{"empty gets bare dir", "", `C:\App`, `C:\App`},
		{"append with separator", `C:\Windows`, `C:\App`, `C:\Windows;C:\App`},
		{"no doubled separator", `C:\Windows;`, `C:\App`, `C:\Windows;C:\App`},
		{"idempotent when present", `C:\Windows;C:\App`, `C:\App`, `C:\Windows;C:\App`},
		{"idempotent case-insensitive", `C:\Windows;C:\app`, `C:\APP`, `C:\Windows;C:\app`},
		{"preserves expand vars verbatim", `%SystemRoot%\system32`, `C:\App`, `%SystemRoot%\system32;C:\App`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, pathAppend(tc.pathVal, tc.dir))
		})
	}
}

func TestPathRemove(t *testing.T) {
	tests := []struct {
		name    string
		pathVal string
		dir     string
		want    string
	}{
		{"remove only entry", `C:\App`, `C:\App`, ""},
		{"remove middle", `C:\Windows;C:\App;C:\Other`, `C:\App`, `C:\Windows;C:\Other`},
		{"remove tail", `C:\Windows;C:\App`, `C:\App`, `C:\Windows`},
		{"case insensitive", `C:\Windows;C:\app`, `C:\APP`, `C:\Windows`},
		{"trailing slash tolerated", `C:\Windows;C:\App\`, `C:\App`, `C:\Windows`},
		{"absent is unchanged", `C:\Windows;C:\Other`, `C:\App`, `C:\Windows;C:\Other`},
		{"drops empty doubled segment", `C:\Windows;;C:\App`, `C:\App`, `C:\Windows`},
		{"keeps other expand vars verbatim", `%SystemRoot%;C:\App`, `C:\App`, `%SystemRoot%`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, pathRemove(tc.pathVal, tc.dir))
		})
	}
}

// Append-then-remove round-trips back to a value with no trace of the dir.
func TestPathAppendRemoveRoundTrip(t *testing.T) {
	base := `%SystemRoot%\system32;C:\Windows`
	dir := `C:\Program Files\MASS`
	added := pathAppend(base, dir)
	require.True(t, pathContains(added, dir))
	removed := pathRemove(added, dir)
	require.False(t, pathContains(removed, dir))
	require.Equal(t, base, removed)
}

func TestInstallDirOf(t *testing.T) {
	// Plain staging: install dir is the exe's directory.
	plain := filepath.Join("opt", "mass", "mass")
	require.Equal(t, filepath.Join("opt", "mass"), installDirOf(plain))

	// macOS .app bundle: install dir is the .app root, not Contents/MacOS.
	bundle := filepath.Join("Applications", "MASS.app", "Contents", "MacOS", "mass")
	require.Equal(t, filepath.Join("Applications", "MASS.app"), installDirOf(bundle))
}
