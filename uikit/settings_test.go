package uikit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogLevelSelect(t *testing.T) {
	out := LogLevelSelect("debug")
	require.Contains(t, out, `data-bind="appLogLevel"`)
	require.Contains(t, out, `value="debug"`)
	require.Contains(t, out, `@post('api/settings')`)
	for _, lvl := range []string{"trace", "debug", "info", "warn", "error"} {
		require.Contains(t, out, `value="`+lvl+`"`)
	}

	// Empty defaults to info.
	require.Contains(t, LogLevelSelect(""), `value="info"`)
}

func TestThemeSelect(t *testing.T) {
	registerTheme(ThemeInfo{Name: "synthwave", Label: "Synthwave", Base: ThemeDark, BG: "#140c28"}, "x")
	t.Cleanup(resetLoadedThemes)

	out := ThemeSelect("light")
	require.Contains(t, out, `data-bind="appTheme"`)
	require.Contains(t, out, `value="light"`)
	require.Contains(t, out, "window.massSetTheme")
	// Built-ins use their theme-sound labels; the loaded theme shows too.
	require.Contains(t, out, `value="dark">Carbon<`)
	require.Contains(t, out, `value="light">Cream<`)
	require.Contains(t, out, `value="synthwave">Synthwave<`)

	require.Contains(t, ThemeSelect(""), `value="dark"`, "empty defaults to dark")
}

func TestThemeSignalSeedsWithoutSelect(t *testing.T) {
	out := ThemeSignal("light")
	require.Contains(t, out, `data-bind="appTheme"`)
	require.Contains(t, out, `value="light"`)
	require.NotContains(t, out, "<sl-select", "ThemeSignal seeds the signal but renders no picker")
}

func TestConnectionSection(t *testing.T) {
	out := ConnectionSection("https://gw.example.com", true, "/etc/ca.pem")
	require.Contains(t, out, `data-bind="appEndpoint"`)
	require.Contains(t, out, `data-bind="appToken"`)
	require.Contains(t, out, `data-bind="appCACert"`)
	require.Contains(t, out, "https://gw.example.com")
	require.Contains(t, out, "/etc/ca.pem")
	require.Contains(t, out, "@post('api/connection')")
	// The token value is never echoed back; only a "leave blank" placeholder.
	require.Contains(t, out, "leave blank to keep")
	require.NotContains(t, out, `value="•`, "the token input must not carry a value")
	// OK / failure badges with short fixed labels (the full reason goes to the
	// log), and an effect that clears them after a moment so the menu stays open.
	require.Contains(t, out, "$appConnOK")
	require.Contains(t, out, "$appConnFail")
	require.Contains(t, out, "Connected")
	require.Contains(t, out, "Connection failed")
	require.Contains(t, out, "data-effect")
	require.Contains(t, out, "setTimeout")
	// The fields autosave (debounced) and the button only verifies.
	require.Contains(t, out, "api/connection/save")
	require.Contains(t, out, "__debounce")
	require.Contains(t, out, ">Verify<")
	require.NotContains(t, out, "Verify &amp; Save")

	// Without a stored token, the placeholder invites pasting one.
	noTok := ConnectionSection("http://localhost:3455", false, "")
	require.Contains(t, noTok, "Paste a token")
}

func TestSettingsShellWrapsItems(t *testing.T) {
	out := SettingsShell("<one></one>", "<two></two>")
	require.Contains(t, out, `name="gear"`, "the shell carries the gear trigger")
	require.Contains(t, out, "<sl-dropdown")
	require.Contains(t, out, "<one></one>")
	require.Contains(t, out, "<two></two>")
	// Items appear in order.
	require.Less(t, strings.Index(out, "<one>"), strings.Index(out, "<two>"))
}
