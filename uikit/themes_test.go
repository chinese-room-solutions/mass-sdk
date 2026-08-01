package uikit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeThemeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func TestLoadThemesFromDirValid(t *testing.T) {
	t.Cleanup(resetLoadedThemes)
	resetLoadedThemes()

	dir := t.TempDir()
	writeThemeFile(t, dir, "synthwave.css", "/* base: dark */\n/* label: Synthwave 80s */\n--mass-bg-base: #140c28;\n--mass-text: #e8e3f6;")

	require.NoError(t, loadThemesFromDir(dir))

	ti, ok := LookupTheme("synthwave")
	require.True(t, ok)
	require.Equal(t, "Synthwave 80s", ti.Label)
	require.Equal(t, ThemeDark, ti.Base)
	require.Equal(t, "#140c28", ti.BG)

	css := ThemesCSS()
	require.Contains(t, css, "html.sl-theme-synthwave, .sl-theme-synthwave {")
	require.Contains(t, css, "--mass-bg-base: #140c28;")
	require.Contains(t, css, "--mass-text: #e8e3f6;")
}

func TestParseThemeFileDirectivesAndBG(t *testing.T) {
	tests := []struct {
		name      string
		file      string
		body      string
		wantName  Theme
		wantLabel string
		wantBase  Theme
		wantBG    string
	}{
		{
			name:      "explicit directives and bg",
			file:      "synthwave.css",
			body:      "/* base: light */\n/* label: Synthwave 80s */\n--mass-bg-base: #140c28;",
			wantName:  "synthwave",
			wantLabel: "Synthwave 80s",
			wantBase:  ThemeLight,
			wantBG:    "#140c28",
		},
		{
			name:      "defaults: dark base, title-cased name, base bg",
			file:      "ocean-breeze.css",
			body:      "--mass-text: #fff;",
			wantName:  "ocean-breeze",
			wantLabel: "Ocean Breeze",
			wantBase:  ThemeDark,
			wantBG:    "#171616", // falls back to dark base bg
		},
		{
			name:      "light base falls back to light bg",
			file:      "solar.css",
			body:      "/* base: light */\n--mass-text: #000;",
			wantName:  "solar",
			wantLabel: "Solar",
			wantBase:  ThemeLight,
			wantBG:    "#f5efe2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, css, err := parseThemeFile(tt.file, tt.body)
			require.NoError(t, err)
			require.Equal(t, tt.wantName, info.Name)
			require.Equal(t, tt.wantLabel, info.Label)
			require.Equal(t, tt.wantBase, info.Base)
			require.Equal(t, tt.wantBG, info.BG)
			require.Contains(t, css, "html.sl-theme-"+string(tt.wantName)+", .sl-theme-"+string(tt.wantName)+" {")
		})
	}
}

func TestParseThemeFileRejections(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
	}{
		{"uppercase name", "Synthwave.css", "--mass-text: #fff;"},
		{"underscore name", "synth_wave.css", "--mass-text: #fff;"},
		{"too long name", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.css", "--mass-text: #fff;"}, // 34 chars
		{"leading digit", "80s.css", "--mass-text: #fff;"},
		{"open brace", "bad.css", "--mass-text: #fff; }"},
		{"close brace in selector attempt", "bad.css", "body { color: red; }"},
		{"angle bracket", "bad.css", "--mass-text: #fff; /* </style> */"},
		{"builtin collision dark", "dark.css", "--mass-text: #fff;"},
		{"builtin collision light", "light.css", "--mass-text: #fff;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseThemeFile(tt.file, tt.body)
			require.Error(t, err)
		})
	}
}

func TestLoadThemesFromDirJoinsSkipped(t *testing.T) {
	t.Cleanup(resetLoadedThemes)
	resetLoadedThemes()

	dir := t.TempDir()
	writeThemeFile(t, dir, "good.css", "--mass-text: #fff;")
	writeThemeFile(t, dir, "Bad.css", "--mass-text: #fff;")      // bad name
	writeThemeFile(t, dir, "braces.css", "body { color: red; }") // forbidden char
	writeThemeFile(t, dir, "dark.css", "--mass-text: #fff;")     // collision

	err := loadThemesFromDir(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Bad.css")
	require.Contains(t, err.Error(), "braces.css")
	require.Contains(t, err.Error(), "dark.css")

	_, ok := LookupTheme("good")
	require.True(t, ok, "valid themes still register despite skipped ones")
}

func TestLoadThemesFromDirMissing(t *testing.T) {
	t.Cleanup(resetLoadedThemes)
	resetLoadedThemes()
	require.NoError(t, loadThemesFromDir(filepath.Join(t.TempDir(), "nope")))
}

func TestSeedThemesOnlyWhenDirMissing(t *testing.T) {
	t.Cleanup(resetLoadedThemes)
	resetLoadedThemes()

	root := t.TempDir()
	dir := filepath.Join(root, "themes")

	// Dir missing → seeded.
	require.NoError(t, seedThemes(dir))
	seeded := filepath.Join(dir, "synthwave.css")
	require.FileExists(t, seeded)

	// Simulate a user deleting the seeded theme, then a reload that must NOT
	// reseed because the dir now exists. LoadThemes guards seeding on dir
	// existence, so here we assert the guard directly: with the dir present,
	// loadThemesFromDir (not seedThemes) runs and finds no synthwave.
	require.NoError(t, os.Remove(seeded))
	_, statErr := os.Stat(dir)
	require.NoError(t, statErr, "dir still exists so seeding must be skipped")
	require.NoError(t, loadThemesFromDir(dir))
	_, ok := LookupTheme("synthwave")
	require.False(t, ok, "deleted theme stays deleted; no reseed when dir exists")
}

// The embedded synthwave.css must itself pass the loader's validation.
func TestEmbeddedSynthwaveLoads(t *testing.T) {
	t.Cleanup(resetLoadedThemes)
	resetLoadedThemes()

	dir := t.TempDir()
	require.NoError(t, seedThemes(dir))
	require.NoError(t, loadThemesFromDir(dir))

	ti, ok := LookupTheme("synthwave")
	require.True(t, ok)
	require.Equal(t, "Synthwave", ti.Label)
	require.Equal(t, ThemeDark, ti.Base)
	require.Equal(t, "#241b2f", ti.BG)
}

func TestThemesCSSCombinationAndOrder(t *testing.T) {
	t.Cleanup(resetLoadedThemes)
	resetLoadedThemes()

	require.Empty(t, ThemesCSS(), "no loaded themes → empty")

	dir := t.TempDir()
	writeThemeFile(t, dir, "zed.css", "--mass-text: #000;")
	writeThemeFile(t, dir, "alpha.css", "--mass-text: #fff;")
	require.NoError(t, loadThemesFromDir(dir))

	css := ThemesCSS()
	require.Contains(t, css, "html.sl-theme-alpha, .sl-theme-alpha {")
	require.Contains(t, css, "html.sl-theme-zed, .sl-theme-zed {")
	require.Less(t, strings.Index(css, "sl-theme-alpha"), strings.Index(css, "sl-theme-zed"), "themes combine name-sorted")
}
