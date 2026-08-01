package uikit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A theme installed after a process's startup LoadThemes must still resolve:
// ParseTheme re-reads the shared dir on an unknown non-empty name, so a
// long-running module process (runtime gateway) picks up live installs.
func TestParseThemeReloadsOnUnknownName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(resetLoadedThemes)

	require.NoError(t, LoadThemes())
	_, ok := LookupTheme("late-arrival")
	require.False(t, ok)

	cfg, err := os.UserConfigDir()
	require.NoError(t, err)
	css := []byte("/* label: Late Arrival */\n/* base: dark */\n--mass-bg-base: #101015;\n")
	require.NoError(t, os.WriteFile(filepath.Join(cfg, "mass", "themes", "late-arrival.css"), css, 0o644))

	require.Equal(t, Theme("late-arrival"), ParseTheme("late-arrival"))
	require.Contains(t, ThemesCSS(), "html.sl-theme-late-arrival, .sl-theme-late-arrival {")

	require.Equal(t, ThemeDark, ParseTheme("still-unknown"))
	require.Equal(t, ThemeDark, ParseTheme(""))
}
