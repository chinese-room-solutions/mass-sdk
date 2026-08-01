package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/chinese-room-solutions/mass-sdk/fsutil"
	"github.com/chinese-room-solutions/mass-sdk/uikit"
	"github.com/rs/zerolog"
)

// AppConfig is a standalone app's persisted settings. Apps store it in their
// own directory (e.g. the singleton dir) and seed the settings dropdown from
// it. Fields are optional; empty means "use the default".
type AppConfig struct {
	LogLevel string `json:"logLevel,omitempty"`
	Theme    string `json:"theme,omitempty"`
}

const (
	configFileName = "config.json"

	// DefaultLogLevel and DefaultTheme are applied when the stored value is
	// empty, so a fresh app starts quiet (info) and dark.
	DefaultLogLevel = "info"
	DefaultTheme    = "dark"
)

// LoadConfig reads the app config from dir, returning defaults when the file
// is absent or unreadable (a corrupt config shouldn't stop the app).
func LoadConfig(dir string) AppConfig {
	cfg := AppConfig{LogLevel: DefaultLogLevel, Theme: DefaultTheme}
	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultLogLevel
	}
	if cfg.Theme == "" {
		cfg.Theme = DefaultTheme
	}
	return cfg
}

// SaveConfig persists the app config to dir, creating it if needed.
func SaveConfig(dir string, cfg AppConfig) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	return fsutil.WriteFileAtomic(filepath.Join(dir, configFileName), data, 0o600)
}

// OpenLogFile opens (creating/appending) a log file named name inside dir.
// The caller owns closing it.
func OpenLogFile(dir, name string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	return f, nil
}

// ApplyLogLevel sets the global zerolog level from a level name, falling back
// to DefaultLogLevel on an empty or unparseable value.
func ApplyLogLevel(name string) {
	lvl, err := zerolog.ParseLevel(name)
	if err != nil || name == "" {
		lvl, _ = zerolog.ParseLevel(DefaultLogLevel)
	}
	zerolog.SetGlobalLevel(lvl)
}

// Settings backs the POST /api/settings endpoint the settings controls post to
// (see uikit's LogLevelSelect / ThemeSelect). Construct with [NewSettings], mount
// [Settings.Handler], and — once the native window exists — register
// [Settings.SetOnThemeChange] so a theme change also repaints the window chrome.
type Settings struct {
	dir    string
	logger zerolog.Logger

	mu            sync.RWMutex
	onThemeChange func(theme string)
}

// NewSettings creates a Settings bound to the app's config dir.
func NewSettings(dir string, logger zerolog.Logger) *Settings {
	return &Settings{dir: dir, logger: logger}
}

// SetOnThemeChange registers a callback fired when the theme changes (e.g.
// the native window's SetTheme). Safe to call after the server is serving;
// the page can't change the theme before the window exists anyway.
func (s *Settings) SetOnThemeChange(fn func(theme string)) {
	s.mu.Lock()
	s.onThemeChange = fn
	s.mu.Unlock()
}

// Handler applies the log level immediately (zerolog is global), persists
// both log level and theme so they survive a restart, and fires the
// theme-change callback so the native chrome follows. Empty fields are left
// unchanged. Mount at uikit.SettingsPath; the signal names parsed here are the
// same uikit.SignalApp* constants the settings controls bind.
func (s *Settings) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var signals map[string]any
		if err := json.NewDecoder(r.Body).Decode(&signals); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		logLevel, _ := signals[uikit.SignalAppLogLevel].(string)
		theme, _ := signals[uikit.SignalAppTheme].(string)

		cfg := LoadConfig(s.dir)
		if logLevel != "" {
			ApplyLogLevel(logLevel)
			cfg.LogLevel = logLevel
		}
		if theme != "" {
			if _, ok := uikit.LookupTheme(theme); !ok {
				s.logger.Warn().Str("theme", theme).Msg("ignoring unknown theme")
			} else {
				cfg.Theme = theme
				s.mu.RLock()
				cb := s.onThemeChange
				s.mu.RUnlock()
				if cb != nil {
					cb(theme)
				}
			}
		}
		if err := SaveConfig(s.dir, cfg); err != nil {
			s.logger.Warn().Err(err).Msg("persisting settings")
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
