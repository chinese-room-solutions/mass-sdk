package uikit

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
)

// Theme is the stable UI theme identifier shared by uikit renderers, gui
// handlers, and the apps between them. Built-ins are "dark"/"light"; pluggable
// themes carry their file name.
type Theme string

const (
	ThemeDark  Theme = "dark"
	ThemeLight Theme = "light"
)

// ThemeInfo describes one registered theme.
type ThemeInfo struct {
	Name  Theme  // stable id: config value, ?theme=, $theme signal, CSS class suffix
	Label string // user-visible label ("Carbon", "Cream", "Synthwave", ...)
	Base  Theme  // ThemeDark | ThemeLight: color-scheme, Shoelace class, native chrome
	BG    string // page bg hex for the critical first-paint style
}

// builtinThemes are the two always-present themes. Anything else is loaded from
// the shared themes dir by LoadThemes. BG mirrors --mass-bg-base of the theme's
// block in theme.css — keep them in sync.
var builtinThemes = []ThemeInfo{
	{Name: ThemeDark, Label: "Carbon", Base: ThemeDark, BG: "#171616"},
	{Name: ThemeLight, Label: "Cream", Base: ThemeLight, BG: "#f5efe2"},
}

// themeRegistry holds the loaded (pluggable) themes, keyed by name. Built-ins
// are not stored here — they are always merged in by the accessors.
var (
	themeMu       sync.RWMutex
	loadedThemes  = map[Theme]ThemeInfo{}
	loadedThemeSt = map[Theme]string{} // name → wrapped CSS block
)

// Themes returns every registered theme: the built-ins first (dark, light),
// then the loaded pluggable themes sorted by name.
func Themes() []ThemeInfo {
	themeMu.RLock()
	loaded := make([]ThemeInfo, 0, len(loadedThemes))
	for _, ti := range loadedThemes {
		loaded = append(loaded, ti)
	}
	themeMu.RUnlock()

	sort.Slice(loaded, func(i, j int) bool { return loaded[i].Name < loaded[j].Name })

	out := make([]ThemeInfo, 0, len(builtinThemes)+len(loaded))
	out = append(out, builtinThemes...)
	out = append(out, loaded...)
	return out
}

// LookupTheme returns the ThemeInfo registered under name (built-in or loaded),
// and whether it was found.
func LookupTheme(name string) (ThemeInfo, bool) {
	for _, ti := range builtinThemes {
		if string(ti.Name) == name {
			return ti, true
		}
	}
	themeMu.RLock()
	defer themeMu.RUnlock()
	ti, ok := loadedThemes[Theme(name)]
	return ti, ok
}

// ParseTheme normalizes a theme name at a read seam: a registered theme maps to
// itself, anything else (missing, unknown) falls back to ThemeDark. An unknown
// non-empty name re-reads the shared themes dir once before giving up: themes
// install live now, and a long-running module process (a runtime gateway
// serving ?theme= pages) would otherwise render a theme installed after its
// startup as the dark fallback until restarted.
func ParseTheme(name string) Theme {
	if ti, ok := LookupTheme(name); ok {
		return ti.Name
	}
	if name != "" {
		if err := LoadThemes(); err == nil {
			if ti, ok := LookupTheme(name); ok {
				return ti.Name
			}
		}
	}
	return ThemeDark
}

// ThemesJSON returns a name-keyed map of {base,label} for every registered
// theme, as a compact JSON object for injection into the page (window.__massThemes).
func ThemesJSON() string {
	type entry struct {
		Base  string `json:"base"`
		Label string `json:"label"`
	}
	m := map[string]entry{}
	for _, ti := range Themes() {
		m[string(ti.Name)] = entry{Base: string(ti.Base), Label: ti.Label}
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// HTMLClass returns the <html> class list carrying this theme: the base
// Shoelace class (sl-theme-dark|light, so vendored Shoelace theme CSS keeps
// working) plus an sl-theme-<name> overlay class for non-base (pluggable)
// themes. An unknown theme falls back to "sl-theme-dark".
func (t Theme) HTMLClass() string {
	ti, ok := LookupTheme(string(t))
	if !ok {
		return "sl-theme-dark"
	}
	base := "sl-theme-" + string(ti.Base)
	if ti.Name == ti.Base {
		return base
	}
	return base + " sl-theme-" + string(ti.Name)
}

// themeQueryParam is the query parameter module pages receive their theme in.
const themeQueryParam = "theme"

// ThemeFromRequest implements the ?theme= convention: module UIs are served
// full pages inside an iframe whose src carries the host's theme as
// "?theme=$theme". A registered theme name maps to itself; anything else
// (including a missing parameter or an unknown name) is ThemeDark, the default.
func ThemeFromRequest(r *http.Request) Theme {
	return ParseTheme(r.URL.Query().Get(themeQueryParam))
}

// The gui↔uikit contract: the settings controls rendered here post to fixed
// endpoints with fixed Datastar signal names, and the gui package's handlers
// parse the same names. Both sides — and the apps that mount the handlers —
// reference these constants so the pairing can't drift.
const (
	// SettingsPath is the endpoint LogLevelSelect / ThemeSelect post to; mount
	// gui.Settings.Handler there.
	SettingsPath = "api/settings"
	// ConnectionPath is the endpoint ConnectionSection's Verify button posts to;
	// mount gui.ConnectionHandler there.
	ConnectionPath = "api/connection"
	// ConnectionSavePath is the endpoint the connection fields autosave to
	// (debounced); mount gui.ConnectionSaveHandler there.
	ConnectionSavePath = "api/connection/save"

	// Signal names bound by the settings controls and read by gui's handlers.
	SignalAppLogLevel = "appLogLevel"
	SignalAppTheme    = "appTheme"
	SignalAppEndpoint = "appEndpoint"
	SignalAppToken    = "appToken"
	SignalAppCACert   = "appCACert"
)
