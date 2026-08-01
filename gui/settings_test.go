package gui

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_DefaultsWhenAbsent(t *testing.T) {
	cfg := LoadConfig(t.TempDir()) // empty dir, no config.json
	require.Equal(t, DefaultLogLevel, cfg.LogLevel)
	require.Equal(t, DefaultTheme, cfg.Theme)
}

func TestSaveLoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := AppConfig{LogLevel: "debug", Theme: "light"}
	require.NoError(t, SaveConfig(dir, want))
	require.Equal(t, want, LoadConfig(dir))
}

func TestLoadConfig_EmptyFieldsFallBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, SaveConfig(dir, AppConfig{})) // both fields empty
	cfg := LoadConfig(dir)
	require.Equal(t, DefaultLogLevel, cfg.LogLevel)
	require.Equal(t, DefaultTheme, cfg.Theme)
}

// The settings handler persists the theme only when it is a registered one;
// an unknown theme is ignored (logged as a warning) and never persisted.
func TestSettingsHandler_ThemeValidation(t *testing.T) {
	tests := []struct {
		name      string
		theme     string
		wantTheme string
	}{
		{"known dark", "dark", "dark"},
		{"known light", "light", "light"},
		{"unknown ignored", "sepia", DefaultTheme},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			s := NewSettings(dir, zerolog.Nop())
			var gotCB string
			s.SetOnThemeChange(func(theme string) { gotCB = theme })

			body := `{"appTheme":"` + tt.theme + `"}`
			r := httptest.NewRequest("POST", "/api/settings", strings.NewReader(body))
			w := httptest.NewRecorder()
			s.Handler()(w, r)

			require.Equal(t, tt.wantTheme, LoadConfig(dir).Theme)
			if tt.theme == tt.wantTheme {
				require.Equal(t, tt.theme, gotCB, "known theme fires the callback")
			} else {
				require.Empty(t, gotCB, "unknown theme must not fire the callback")
			}
		})
	}
}

func TestApplyLogLevel(t *testing.T) {
	prev := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	tests := []struct {
		name string
		in   string
		want zerolog.Level
	}{
		{name: "valid level", in: "warn", want: zerolog.WarnLevel},
		{name: "empty falls back to default", in: "", want: zerolog.InfoLevel},
		{name: "garbage falls back to default", in: "nonsense", want: zerolog.InfoLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ApplyLogLevel(tt.in)
			require.Equal(t, tt.want, zerolog.GlobalLevel())
		})
	}
}
