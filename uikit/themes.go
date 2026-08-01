package uikit

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/fsutil"
)

// themesFS embeds the seeded example theme(s) shipped with the SDK. On first run
// LoadThemes copies these into the shared themes dir as editable templates.
//
//go:embed themes/*.css
var themesFS embed.FS

// themesSubdir is the subdirectory (under os.UserConfigDir()) holding
// pluggable theme .css files. Matches connstore's shared-dir layout.
const themesSubdir = "mass/themes"

// themeNameRe constrains a theme file name (sans .css), which doubles as the
// CSS class suffix: lowercase-start, then lowercase/digit/dash, up to 32 chars.
var themeNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// baseDirectiveRe and labelDirectiveRe extract the optional theme directives
// from anywhere in a theme file's comments.
var (
	baseDirectiveRe  = regexp.MustCompile(`/\*\s*base:\s*(dark|light)\s*\*/`)
	labelDirectiveRe = regexp.MustCompile(`/\*\s*label:\s*([^*\n]+?)\s*\*/`)
	bgDeclRe         = regexp.MustCompile(`--mass-bg-base:\s*([^;]+);`)
)

// LoadThemes seeds the shared themes dir with the embedded example on first run
// (only when the dir doesn't exist — so deleting a theme sticks), then scans
// os.UserConfigDir()/mass/themes/*.css and registers every valid theme. Call
// once at startup; it returns a joined error describing every skipped file, which
// callers should log as a warning (a bad theme file must not stop the app).
func LoadThemes() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locating user config dir: %w", err)
	}
	dir := filepath.Join(configDir, filepath.FromSlash(themesSubdir))

	if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
		if seedErr := seedThemes(dir); seedErr != nil {
			return fmt.Errorf("seeding themes dir: %w", seedErr)
		}
	}
	return loadThemesFromDir(dir)
}

// seedThemes creates dir and writes every embedded example theme into it. Called
// only when dir doesn't exist yet, so a user deleting a seeded theme sticks.
func seedThemes(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating themes dir: %w", err)
	}
	entries, err := themesFS.ReadDir("themes")
	if err != nil {
		return fmt.Errorf("reading embedded themes: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".css") {
			continue
		}
		data, rerr := themesFS.ReadFile("themes/" + e.Name())
		if rerr != nil {
			return fmt.Errorf("reading embedded theme %s: %w", e.Name(), rerr)
		}
		if werr := fsutil.WriteFileAtomic(filepath.Join(dir, e.Name()), data, 0o644); werr != nil {
			return fmt.Errorf("writing seeded theme %s: %w", e.Name(), werr)
		}
	}
	return nil
}

// loadThemesFromDir scans dir for *.css theme files, validates and registers the
// valid ones, and returns a joined error covering every skipped file. A missing
// dir is not an error (there is simply nothing to load). Split from LoadThemes so
// tests can drive it with a t.TempDir.
func loadThemesFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading themes dir %s: %w", dir, err)
	}

	var skipped []error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".css") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			skipped = append(skipped, fmt.Errorf("reading %s: %w", e.Name(), rerr))
			continue
		}
		info, css, perr := parseThemeFile(e.Name(), string(data))
		if perr != nil {
			skipped = append(skipped, fmt.Errorf("skipping %s: %w", e.Name(), perr))
			continue
		}
		registerTheme(info, css)
	}
	return errors.Join(skipped...)
}

// parseThemeFile validates a theme file and returns its ThemeInfo plus the CSS
// block wrapped in the overlay selector. fileName is the base name including
// ".css"; content is the raw file body.
func parseThemeFile(fileName, content string) (ThemeInfo, string, error) {
	name := strings.TrimSuffix(fileName, ".css")
	if !themeNameRe.MatchString(name) {
		return ThemeInfo{}, "", fmt.Errorf("invalid theme name %q: must match %s", name, themeNameRe.String())
	}
	for _, ti := range builtinThemes {
		if string(ti.Name) == name {
			return ThemeInfo{}, "", fmt.Errorf("theme name %q collides with a built-in theme", name)
		}
	}
	// Declarations and comments only: braces would let a file escape its wrapper
	// selector, and '<' would allow a </style> breakout once inlined into the page.
	// color-mix() parens are fine; only these three characters are forbidden.
	if i := strings.IndexAny(content, "{}<"); i >= 0 {
		return ThemeInfo{}, "", fmt.Errorf("theme body contains a forbidden character %q (only CSS declarations and comments are allowed)", content[i])
	}

	base := ThemeDark
	if m := baseDirectiveRe.FindStringSubmatch(content); m != nil {
		base = Theme(m[1])
	}
	baseInfo, _ := LookupTheme(string(base))

	label := titleCaseName(name)
	if m := labelDirectiveRe.FindStringSubmatch(content); m != nil {
		label = strings.TrimSpace(m[1])
	}

	bg := baseInfo.BG
	if m := bgDeclRe.FindStringSubmatch(content); m != nil {
		bg = strings.TrimSpace(m[1])
	}

	info := ThemeInfo{Name: Theme(name), Label: label, Base: base, BG: bg}
	// Two selectors: the html one for the page root, the bare class for elements
	// the app re-stamps with theme classes (MASS marks every sl-dialog). With
	// only the html selector, a stamped element's .sl-theme-dark|light base
	// class would win the builtin token scale back for its whole subtree —
	// ThemesCSS is inlined after the base stylesheets, so the bare class wins
	// that equal-specificity tie by source order.
	wrapped := fmt.Sprintf("html.sl-theme-%s, .sl-theme-%s {\n%s\n}", name, name, strings.TrimRight(content, "\n"))
	return info, wrapped, nil
}

// registerTheme stores a loaded theme's info and wrapped CSS, replacing any
// previously loaded theme of the same name.
func registerTheme(info ThemeInfo, css string) {
	themeMu.Lock()
	loadedThemes[info.Name] = info
	loadedThemeSt[info.Name] = css
	themeMu.Unlock()
}

// Theme install/remove sentinels, for callers mapping to transport errors.
var (
	// ErrThemeBuiltin is returned when removing a built-in theme.
	ErrThemeBuiltin = errors.New("built-in themes cannot be removed")
	// ErrThemeNotInstalled is returned when removing a theme that isn't loaded.
	ErrThemeNotInstalled = errors.New("theme not installed")
)

// InstallTheme validates raw theme CSS and installs it as <name>.css in the
// shared themes dir (creating it if needed), registering the theme live — the
// running app serves it on the next render, no restart. Installing an existing
// name overwrites it: that's the update path. The CSS must satisfy the theme
// contract (declarations and comments only; see parseThemeFile).
func InstallTheme(name string, css []byte) (ThemeInfo, error) {
	info, wrapped, err := parseThemeFile(name+".css", string(css))
	if err != nil {
		return ThemeInfo{}, err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ThemeInfo{}, fmt.Errorf("locating user config dir: %w", err)
	}
	dir := filepath.Join(configDir, filepath.FromSlash(themesSubdir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ThemeInfo{}, fmt.Errorf("creating themes dir: %w", err)
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(dir, name+".css"), css, 0o644); err != nil {
		return ThemeInfo{}, fmt.Errorf("writing theme %s: %w", name, err)
	}
	registerTheme(info, wrapped)
	return info, nil
}

// RemoveTheme deletes an installed pluggable theme's file from the shared
// themes dir and unregisters it live. Built-ins are refused; a name that isn't
// a loaded theme is ErrThemeNotInstalled.
func RemoveTheme(name string) error {
	for _, ti := range builtinThemes {
		if string(ti.Name) == name {
			return fmt.Errorf("%w: %s", ErrThemeBuiltin, name)
		}
	}
	themeMu.Lock()
	_, ok := loadedThemes[Theme(name)]
	if ok {
		delete(loadedThemes, Theme(name))
		delete(loadedThemeSt, Theme(name))
	}
	themeMu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrThemeNotInstalled, name)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locating user config dir: %w", err)
	}
	path := filepath.Join(configDir, filepath.FromSlash(themesSubdir), name+".css")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing theme file: %w", err)
	}
	return nil
}

// resetLoadedThemes clears the loaded-theme registry. For tests, so registry
// state can't bleed between cases.
func resetLoadedThemes() {
	themeMu.Lock()
	loadedThemes = map[Theme]ThemeInfo{}
	loadedThemeSt = map[Theme]string{}
	themeMu.Unlock()
}

// ThemesCSS returns the combined wrapped CSS of all loaded pluggable themes,
// ordered by name, for inlining after the built-in theme CSS. Empty when no
// pluggable themes are loaded.
func ThemesCSS() string {
	themeMu.RLock()
	names := make([]string, 0, len(loadedThemeSt))
	for name := range loadedThemeSt {
		names = append(names, string(name))
	}
	blocks := make([]string, 0, len(loadedThemeSt))
	sort.Strings(names)
	for _, name := range names {
		blocks = append(blocks, loadedThemeSt[Theme(name)])
	}
	themeMu.RUnlock()
	return strings.Join(blocks, "\n\n")
}

// titleCaseName turns a theme name into a default label: dashes become spaces
// and each word is title-cased ("synthwave-80s" → "Synthwave 80s").
func titleCaseName(name string) string {
	words := strings.Split(name, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
