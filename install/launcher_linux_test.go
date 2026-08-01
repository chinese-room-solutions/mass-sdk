//go:build linux

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateLauncher_Linux(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	// A source icon to install into the theme.
	srcIcon := filepath.Join(t.TempDir(), "icon.png")
	require.NoError(t, os.WriteFile(srcIcon, []byte("PNGDATA"), 0o644))

	a := AppSpec{Name: "mass", DisplayName: "MASS", ExeName: "mass"}
	path, err := a.createLauncher(LauncherSpec{
		ExePath:  "/opt/mass/mass",
		IconPath: srcIcon,
		PerUser:  true,
	})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dataHome, "applications", "mass.desktop"), path)

	entry, err := os.ReadFile(path)
	require.NoError(t, err)
	got := string(entry)
	require.Contains(t, got, "Name=MASS")
	require.Contains(t, got, `Exec="/opt/mass/mass"`)
	require.Contains(t, got, "Icon=mass")
	require.Contains(t, got, "StartupWMClass=mass")
	require.Contains(t, got, "Terminal=false")

	// Icon installed into every standard hicolor size under the stable name —
	// the menu draws ~48px, so a 512-only entry would resolve empty and render
	// blank. Spot-check the small (menu) size and the large source size.
	for _, sz := range []string{"48x48", "512x512"} {
		icon, err := os.ReadFile(filepath.Join(
			dataHome, "icons", "hicolor", sz, "apps", "mass.png"))
		require.NoError(t, err, "icon missing at %s", sz)
		require.Equal(t, "PNGDATA", string(icon))
	}

	// A per-user hicolor tree has no index.theme of its own, and without one the
	// freedesktop/KDE icon loader won't resolve the name at all — the launcher
	// renders blank despite the PNGs being present. installIcon must write one
	// that declares the app buckets it populated.
	index, err := os.ReadFile(filepath.Join(dataHome, "icons", "hicolor", "index.theme"))
	require.NoError(t, err, "index.theme not written")
	idx := string(index)
	require.Contains(t, idx, "[Icon Theme]")
	require.Contains(t, idx, "48x48/apps")
	require.Contains(t, idx, "[512x512/apps]")
}

func TestRemoveLauncher_Linux(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	a := AppSpec{Name: "mass", DisplayName: "MASS", ExeName: "mass"}

	srcIcon := filepath.Join(t.TempDir(), "icon.png")
	require.NoError(t, os.WriteFile(srcIcon, []byte("x"), 0o644))
	_, err := a.createLauncher(LauncherSpec{ExePath: "/opt/mass/mass", IconPath: srcIcon, PerUser: true})
	require.NoError(t, err)

	require.NoError(t, a.removeLauncher(true))
	_, err = os.Stat(filepath.Join(dataHome, "applications", "mass.desktop"))
	require.ErrorIs(t, err, os.ErrNotExist)

	// The icon is removed from every size bucket, not just one.
	for _, sz := range []string{"48x48", "512x512"} {
		_, err = os.Stat(filepath.Join(dataHome, "icons", "hicolor", sz, "apps", "mass.png"))
		require.ErrorIs(t, err, os.ErrNotExist, "icon left behind at %s", sz)
	}

	// removeLauncher again on a clean tree is not an error (idempotent).
	require.NoError(t, a.removeLauncher(true))
}
