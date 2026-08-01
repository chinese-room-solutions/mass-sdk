package uikit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstallRemoveTheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(resetLoadedThemes)

	css := []byte("/* label: Neon Test */\n/* base: dark */\n--mass-bg-base: #101015;\n")
	info, err := InstallTheme("neon-test", css)
	require.NoError(t, err)
	require.Equal(t, "Neon Test", info.Label)
	require.Equal(t, ThemeDark, info.Base)
	require.Equal(t, "#101015", info.BG)

	got, ok := LookupTheme("neon-test")
	require.True(t, ok)
	require.Equal(t, info, got)
	require.Contains(t, ThemesCSS(), "html.sl-theme-neon-test, .sl-theme-neon-test {")

	cfg, err := os.UserConfigDir()
	require.NoError(t, err)
	path := filepath.Join(cfg, "mass", "themes", "neon-test.css")
	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, css, onDisk)

	// Reinstall overwrites — the update path, not a conflict.
	_, err = InstallTheme("neon-test", []byte("/* label: Neon Two */\n--mass-bg-base: #000;\n"))
	require.NoError(t, err)
	got, _ = LookupTheme("neon-test")
	require.Equal(t, "Neon Two", got.Label)

	require.NoError(t, RemoveTheme("neon-test"))
	_, ok = LookupTheme("neon-test")
	require.False(t, ok)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestInstallThemeRejectsInvalid(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(resetLoadedThemes)

	tests := []struct {
		name  string
		theme string
		css   string
	}{
		{"braces", "bad-braces", "body { color: red }"},
		{"angle bracket", "bad-angle", "--x: 1; </style>"},
		{"bad name", "Bad_Name", "--x: 1;"},
		{"builtin collision", "dark", "--x: 1;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := InstallTheme(tt.theme, []byte(tt.css))
			require.Error(t, err)
		})
	}
}

func TestRemoveThemeRefusals(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(resetLoadedThemes)

	require.ErrorIs(t, RemoveTheme("dark"), ErrThemeBuiltin)
	require.ErrorIs(t, RemoveTheme("ghost"), ErrThemeNotInstalled)
}
