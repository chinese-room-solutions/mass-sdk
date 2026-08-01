//go:build linux

package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// createLauncher writes a freedesktop .desktop entry so the app appears in the
// application menu / launcher, installs the icon into the icon theme so the
// entry isn't blank, and refreshes the desktop database so the menu picks it up
// without a re-login. Per-user → ~/.local/share; machine-wide → /usr/share
// (needs root).
func (a AppSpec) createLauncher(spec LauncherSpec) (string, error) {
	dataDir, err := a.xdgDataDir(spec.PerUser)
	if err != nil {
		return "", err
	}
	appsDir := filepath.Join(dataDir, "applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(appsDir, a.Name+".desktop")

	// Install the PNG icon into the hicolor theme under the stable name <Name>,
	// then reference it by that name (not an absolute path) so the icon theme
	// and HiDPI scaling resolve it. If no icon was provided, fall back to the
	// themed name anyway — a missing icon just renders generic.
	icon := a.Name
	if spec.IconPath != "" {
		if err := a.installIcon(dataDir, spec.IconPath); err != nil {
			return "", err
		}
		// Refresh the hicolor icon cache so KDE/GNOME's icon loader sees the new
		// PNG immediately. update-desktop-database (below) only refreshes the
		// .desktop database — a stale icon-theme.cache leaves the entry blank.
		updateIconCache(filepath.Join(dataDir, "icons", "hicolor"))
	}

	// StartupWMClass ties the running window (whose WM class GTK sets from the
	// program name) back to this launcher, so the taskbar groups it under our
	// icon instead of spawning a second generic entry.
	entry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec="%s"
Icon=%s
Terminal=false
Categories=Utility;
StartupNotify=true
StartupWMClass=%s
`, a.DisplayName, spec.ExePath, icon, a.ExeName)

	if err := os.WriteFile(path, []byte(entry), 0o644); err != nil {
		return "", err
	}
	updateDesktopDatabase(appsDir)
	return path, nil
}

func (a AppSpec) removeLauncher(perUser bool) error {
	dataDir, err := a.xdgDataDir(perUser)
	if err != nil {
		return err
	}
	appsDir := filepath.Join(dataDir, "applications")
	path := filepath.Join(appsDir, a.Name+".desktop")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, sz := range iconSizes {
		dim := fmt.Sprintf("%dx%d", sz, sz)
		iconPath := filepath.Join(dataDir, "icons", "hicolor", dim, "apps", a.Name+".png")
		if err := os.Remove(iconPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	updateDesktopDatabase(appsDir)
	updateIconCache(filepath.Join(dataDir, "icons", "hicolor"))
	return nil
}

// iconSizes are the hicolor size buckets the app PNG is installed into. The menu
// looks up an icon at the pixel size it's about to draw (~48 for a launcher row),
// and a raster icon theme only resolves a name at a size that actually has a
// directory — a single 512x512 entry leaves a 48px lookup empty, so the entry
// renders blank. Installing the source into every standard bucket makes the
// lookup hit at any size; the loader downscales the larger source as needed.
var iconSizes = []int{16, 22, 24, 32, 48, 64, 128, 256, 512}

// installIcon copies the app's PNG into the hicolor icon theme under every
// standard size, as <dataDir>/icons/hicolor/<N>x<N>/apps/<Name>.png, so the
// launcher can resolve it at the small size it draws (not just at 512).
func (a AppSpec) installIcon(dataDir, srcIcon string) error {
	data, err := os.ReadFile(srcIcon)
	if err != nil {
		return err
	}
	for _, sz := range iconSizes {
		dim := fmt.Sprintf("%dx%d", sz, sz)
		iconDir := filepath.Join(dataDir, "icons", "hicolor", dim, "apps")
		if err := os.MkdirAll(iconDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(iconDir, a.Name+".png"), data, 0o644); err != nil {
			return err
		}
	}
	return writeHicolorIndex(filepath.Join(dataDir, "icons", "hicolor"))
}

// writeHicolorIndex writes a minimal index.theme for a per-user hicolor tree.
// The system /usr/share/icons/hicolor ships one, but a fresh ~/.local/share
// tree has none — and without it the freedesktop/KDE icon *loader* won't resolve
// a name in that tree at all (the launcher renders blank), even though the PNGs
// and the gtk-update-icon-cache file are present. It only lists the app-icon
// buckets we actually populate; if a system index.theme already governs this
// path we leave it untouched.
func writeHicolorIndex(hicolorDir string) error {
	path := filepath.Join(hicolorDir, "index.theme")
	if _, err := os.Stat(path); err == nil {
		return nil // already present (e.g. a machine-wide /usr/share tree)
	}
	var b strings.Builder
	dirs := make([]string, len(iconSizes))
	for i, sz := range iconSizes {
		dirs[i] = fmt.Sprintf("%dx%d/apps", sz, sz)
	}
	fmt.Fprintf(&b, "[Icon Theme]\nName=Hicolor\nComment=Fallback icon theme\nHidden=true\nDirectories=%s\n\n", strings.Join(dirs, ","))
	for _, sz := range iconSizes {
		fmt.Fprintf(&b, "[%dx%d/apps]\nSize=%d\nContext=Applications\nType=Threshold\n\n", sz, sz, sz)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// updateDesktopDatabase nudges the menu to register the new .desktop without a
// re-login. Best-effort: it's absent on minimal systems and the menu refreshes
// on next scan regardless, so a failure is intentionally ignored.
func updateDesktopDatabase(appsDir string) {
	if bin, err := exec.LookPath("update-desktop-database"); err == nil {
		_ = exec.Command(bin, appsDir).Run()
	}
}

// updateIconCache rebuilds the hicolor theme's icon-theme.cache so the icon
// loader sees a just-installed (or removed) icon without waiting for the cache
// to expire — a stale cache is what renders a launcher entry blank. -f forces a
// rebuild even when the dir mtime looks unchanged; -t tolerates the absence of a
// prior cache. installIcon writes index.theme first (the loader needs it to
// resolve names in a per-user tree — -t only lets the cache build, it does not
// substitute for the theme index). Best-effort: gtk-update-icon-cache is absent
// on minimal systems, and the cache refreshes on next theme scan regardless, so
// a failure is intentionally ignored.
func updateIconCache(hicolorDir string) {
	bin, err := exec.LookPath("gtk-update-icon-cache")
	if err != nil {
		bin, err = exec.LookPath("gtk4-update-icon-cache")
		if err != nil {
			return
		}
	}
	_ = exec.Command(bin, "-f", "-t", hicolorDir).Run()
}

func (a AppSpec) xdgDataDir(perUser bool) (string, error) {
	if !perUser {
		return "/usr/share", nil
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return xdg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}
