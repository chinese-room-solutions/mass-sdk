package uikit

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// registerTestTheme registers a throwaway loaded theme for a single test and
// cleans it up afterward, so registry state can't bleed between tests.
func registerTestTheme(t *testing.T, info ThemeInfo, css string) {
	t.Helper()
	registerTheme(info, css)
	t.Cleanup(resetLoadedThemes)
}

func TestParseTheme(t *testing.T) {
	registerTestTheme(t, ThemeInfo{Name: "synthwave", Label: "Synthwave 80s", Base: ThemeDark, BG: "#140c28"}, "html.sl-theme-synthwave {\n--mass-bg-base: #140c28;\n}")

	tests := []struct {
		name string
		in   string
		want Theme
	}{
		{"dark", "dark", ThemeDark},
		{"light", "light", ThemeLight},
		{"loaded", "synthwave", Theme("synthwave")},
		{"unknown", "sepia", ThemeDark},
		{"empty", "", ThemeDark},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ParseTheme(tt.in))
		})
	}
}

func TestLookupTheme(t *testing.T) {
	registerTestTheme(t, ThemeInfo{Name: "synthwave", Label: "Synthwave 80s", Base: ThemeDark, BG: "#140c28"}, "x")

	tests := []struct {
		name      string
		in        string
		wantOK    bool
		wantLabel string
		wantBase  Theme
	}{
		{"dark", "dark", true, "Carbon", ThemeDark},
		{"light", "light", true, "Cream", ThemeLight},
		{"loaded", "synthwave", true, "Synthwave 80s", ThemeDark},
		{"unknown", "sepia", false, "", ""},
		{"empty", "", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LookupTheme(tt.in)
			require.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				require.Equal(t, tt.wantLabel, got.Label)
				require.Equal(t, tt.wantBase, got.Base)
			}
		})
	}
}

func TestHTMLClass(t *testing.T) {
	registerTestTheme(t, ThemeInfo{Name: "synthwave", Label: "Synthwave 80s", Base: ThemeDark, BG: "#140c28"}, "x")
	registerTheme(ThemeInfo{Name: "solar", Label: "Solar", Base: ThemeLight, BG: "#fff"}, "y")

	tests := []struct {
		name string
		in   Theme
		want string
	}{
		{"dark", ThemeDark, "sl-theme-dark"},
		{"light", ThemeLight, "sl-theme-light"},
		{"loaded dark base", Theme("synthwave"), "sl-theme-dark sl-theme-synthwave"},
		{"loaded light base", Theme("solar"), "sl-theme-light sl-theme-solar"},
		{"unknown", Theme("sepia"), "sl-theme-dark"},
		{"empty", Theme(""), "sl-theme-dark"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.in.HTMLClass())
		})
	}
}

// ThemeFromRequest implements the ?theme= convention: a registered name maps to
// itself, everything else (including a missing parameter) is the dark default.
func TestThemeFromRequest(t *testing.T) {
	registerTestTheme(t, ThemeInfo{Name: "synthwave", Label: "Synthwave 80s", Base: ThemeDark, BG: "#140c28"}, "x")

	tests := []struct {
		name string
		url  string
		want Theme
	}{
		{"no parameter", "/page", ThemeDark},
		{"dark", "/page?theme=dark", ThemeDark},
		{"light", "/page?theme=light", ThemeLight},
		{"loaded", "/page?theme=synthwave", Theme("synthwave")},
		{"garbage", "/page?theme=sepia", ThemeDark},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ThemeFromRequest(httptest.NewRequest("GET", tt.url, nil)))
		})
	}
}

func TestThemesJSON(t *testing.T) {
	registerTestTheme(t, ThemeInfo{Name: "synthwave", Label: "Synthwave 80s", Base: ThemeDark, BG: "#140c28"}, "x")
	out := ThemesJSON()
	require.Contains(t, out, `"dark":{"base":"dark","label":"Carbon"}`)
	require.Contains(t, out, `"light":{"base":"light","label":"Cream"}`)
	require.Contains(t, out, `"synthwave":{"base":"dark","label":"Synthwave 80s"}`)
}

// Layout switches the Shoelace theme class and first-paint bg with the theme, and
// injects the __massThemes registry plus any loaded overlay CSS.
func TestLayoutThemes(t *testing.T) {
	require.Contains(t, Layout("t", "b", ThemeLight), `class="sl-theme-light min-h-screen"`)
	require.Contains(t, Layout("t", "b", ThemeLight), "#f5efe2")
	require.Contains(t, Layout("t", "b", ThemeDark), `class="sl-theme-dark min-h-screen"`)
	require.Contains(t, Layout("t", "b", ThemeDark), "#171616")
	require.Contains(t, Layout("t", "b", ThemeDark), "window.__massThemes")

	registerTestTheme(t, ThemeInfo{Name: "synthwave", Label: "Synthwave 80s", Base: ThemeDark, BG: "#140c28"},
		"html.sl-theme-synthwave {\n--mass-bg-base: #140c28;\n}")
	out := Layout("t", "b", Theme("synthwave"))
	require.Contains(t, out, `class="sl-theme-dark sl-theme-synthwave min-h-screen"`)
	require.Contains(t, out, "#140c28")
	require.Contains(t, out, "html.sl-theme-synthwave {")
	require.Contains(t, out, `"synthwave":{"base":"dark","label":"Synthwave 80s"}`)
}
