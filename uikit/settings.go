package uikit

import (
	"fmt"
	"html"
	"strings"
)

// The settings UI is composable: the SDK provides the dropdown shell and the
// individual reusable controls, but each app assembles its own gear menu from
// them — settings differ from app to app, so the SDK must not own the menu's
// contents. All controls post to SettingsPath to persist; pair with
// gui.Settings.Handler on the server. Endpoints and signal names are the
// exported constants in theme.go — the gui handlers parse the same ones.

// SettingsShell wraps the given control HTML in the standard top-right settings
// affordance: a gear icon button opening a styled dropdown. The items are the
// app's chosen controls (e.g. LogLevelSelect, ThemeSelect, or app-specific ones),
// rendered top to bottom.
func SettingsShell(items ...string) string {
	return fmt.Sprintf(`<sl-dropdown placement="bottom-end" hoist>
	<sl-icon-button slot="trigger" name="gear" label="Settings" title="Settings" style="font-size:1.15rem;color:var(--mass-text-muted)"></sl-icon-button>
	<div style="display:flex;flex-direction:column;gap:0.75rem;padding:0.85rem;width:220px;background:var(--mass-bg-panel);border:1px solid var(--mass-border);border-radius:0.5rem">
		%s
	</div>
</sl-dropdown>`, strings.Join(items, "\n\t\t"))
}

// LogLevelSelect is the Log Level control: a select bound to the app-agnostic
// SignalAppLogLevel signal that applies live and is remembered. It emits its own
// hidden signal input, so it's self-contained — drop it into SettingsShell as-is.
func LogLevelSelect(logLevel string) string {
	if logLevel == "" {
		logLevel = "info"
	}
	return fmt.Sprintf(`<input type="hidden" data-bind="%[1]s" value=%[3]q/>
		<sl-select label="Log Level" size="small" data-attr:value="$%[1]s" data-on:sl-change="$%[1]s = evt.target.value; @post('%[2]s')" title="Applied immediately and remembered.">
			<sl-option value="trace">Trace</sl-option>
			<sl-option value="debug">Debug</sl-option>
			<sl-option value="info">Info</sl-option>
			<sl-option value="warn">Warn</sl-option>
			<sl-option value="error">Error</sl-option>
		</sl-select>`, SignalAppLogLevel, SettingsPath, html.EscapeString(logLevel))
}

// ThemeSelect is the Theme control: a select bound to SignalAppTheme that
// applies live (massSetTheme) and persists. It includes the theme signal seed,
// so use this when the menu shows a theme picker. An app with its own theme
// toggle button (and no picker) should use ThemeSignal instead to seed the
// signal without the select.
func ThemeSelect(theme string) string {
	var opts strings.Builder
	for _, ti := range Themes() {
		fmt.Fprintf(&opts, `<sl-option value=%q>%s</sl-option>`,
			html.EscapeString(string(ti.Name)), html.EscapeString(ti.Label))
	}
	return themeSignalInput(theme) + fmt.Sprintf(`
		<sl-select label="Theme" size="small" data-attr:value="$%[1]s" data-on:sl-change="$%[1]s = evt.target.value; window.massSetTheme($%[1]s); @post('%[2]s')" title="Applied immediately and remembered.">
			%[3]s
		</sl-select>`, SignalAppTheme, SettingsPath, opts.String())
}

// ThemeSignal seeds the appTheme signal without rendering a theme select — for an
// app whose theme is driven by its own toggle button (which binds to appTheme)
// rather than a picker in the settings menu.
func ThemeSignal(theme string) string {
	return themeSignalInput(theme)
}

// ConnectionSection is the MASS connection control for the settings menu: the
// gateway URL, auth token, and an optional custom CA certificate path, with a
// Verify button that checks the gateway accepts them and saves them on success
// (POST api/connection; pair with gui.ConnectionHandler). Apps reach the gateway
// on demand, so there is no persistent connection to "connect" — Verify proves
// the settings work, then stores them. It seeds the appEndpoint/appToken/
// appCACert signals, so drop it straight into SettingsShell.
//
// The token is write-only in the UI: its current value is never rendered back
// into the page. hasToken only controls the placeholder, so the operator can see
// that a token is set without it being exposed; submitting a blank token keeps
// the stored one.
func ConnectionSection(endpoint string, hasToken bool, caCert string) string {
	tokenPlaceholder := "Paste a token"
	if hasToken {
		tokenPlaceholder = "•••••• (leave blank to keep)"
	}
	return fmt.Sprintf(`<input type="hidden" data-bind="%[4]s" value=%[1]q/>
		<input type="hidden" data-bind="%[5]s" value=""/>
		<input type="hidden" data-bind="%[6]s" value=%[2]q/>
		<div data-signals="{appConnBusy:false, appConnOK:false, appConnFail:false}" style="display:none"></div>
		<div style="display:flex;flex-direction:column;gap:0.55rem;border-top:1px solid var(--mass-border);padding-top:0.7rem">
			<span style="font-size:0.72rem;color:var(--mass-text-muted)">MASS connection</span>
			<!-- The fields autosave (debounced) to ConnectionSavePath, like the other
			     settings; the button only verifies the gateway accepts them. -->
			<sl-input label="Gateway URL" size="small" data-bind="%[4]s" autocomplete="off" data-on:sl-input__debounce.600ms="@post('%[8]s')"></sl-input>
			<sl-input label="Auth token" type="password" size="small" toggle-password data-bind="%[5]s" autocomplete="off" placeholder=%[3]q data-on:sl-input__debounce.600ms="@post('%[8]s')"></sl-input>
			<sl-input label="Custom CA cert" size="small" data-bind="%[6]s" autocomplete="off" title="PEM CA bundle for a private-CA gateway; blank uses system trust." data-on:sl-input__debounce.600ms="@post('%[8]s')"></sl-input>
			<!-- One full-width slot: the Verify button, or a green/red badge of the
			     same shape. Both clear themselves after a moment. The badges keep
			     short, fixed labels (the full failure reason goes to the app log);
			     success/danger fills come from the SDK theme tokens. -->
			<div style="display:flex;flex-direction:column" data-effect="($appConnOK || $appConnFail) && setTimeout(() => { $appConnOK = false; $appConnFail = false }, 2400)">
				<sl-button style="width:100%%" data-show="!$appConnOK &amp;&amp; !$appConnFail" size="small" variant="primary" data-attr:loading="$appConnBusy" data-on:click="$appConnFail = false; $appConnOK = false; $appConnBusy = true; @post('%[7]s')" title="Check the gateway accepts these settings">Verify</sl-button>
				<sl-button style="width:100%%;pointer-events:none" tabindex="-1" data-show="$appConnOK" size="small" variant="success"><sl-icon slot="prefix" name="check-circle-fill"></sl-icon>Connected</sl-button>
				<sl-button style="width:100%%;pointer-events:none" tabindex="-1" data-show="$appConnFail" size="small" variant="danger"><sl-icon slot="prefix" name="x-circle-fill"></sl-icon>Connection failed</sl-button>
			</div>
		</div>`,
		html.EscapeString(endpoint),
		html.EscapeString(caCert),
		html.EscapeString(tokenPlaceholder),
		SignalAppEndpoint,
		SignalAppToken,
		SignalAppCACert,
		ConnectionPath,
		ConnectionSavePath,
	)
}

// themeSignalInput is the hidden SignalAppTheme seed shared by ThemeSelect and
// ThemeSignal.
func themeSignalInput(theme string) string {
	if theme == "" {
		theme = "dark"
	}
	return fmt.Sprintf(`<input type="hidden" data-bind="%s" value=%q/>`, SignalAppTheme, html.EscapeString(theme))
}
