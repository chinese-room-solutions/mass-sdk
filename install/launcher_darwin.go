//go:build darwin

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// createLauncher on macOS: a GUI app is launchable simply by being a .app bundle
// in an Applications directory, which the staging already placed (DefaultInstallDir
// / UserInstallDir return the .app path). The only extra is nudging Launch
// Services to register the bundle so it appears in Spotlight/Launchpad promptly;
// that's best-effort (the Finder registers it on next scan regardless).
//
// We register the .app dir that contains the staged exe (…/<App>.app/Contents/
// MacOS/<exe> → the .app root is three levels up when bundled, or the install dir
// itself when staging is flat). We walk up to the nearest ".app" ancestor.
func (a AppSpec) createLauncher(spec LauncherSpec) (string, error) {
	appBundle := appRoot(spec.ExePath)
	// Convert the staged PNG into the bundle's icon. Info.plist names
	// Contents/Resources/icon.icns (CFBundleIconFile=icon); generate it with the
	// macOS-only iconutil. Best-effort: a missing icon just shows the generic app
	// glyph, so failures here never block the install.
	if spec.IconPath != "" && filepath.Ext(appBundle) == ".app" {
		writeMacIcns(appBundle, spec.IconPath)
	}
	// lsregister lives in CoreServices; -f forces (re)registration. Best-effort:
	// ignore the error (the app still launches; Finder registers it on scan).
	const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	_ = exec.Command(lsregister, "-f", appBundle).Run()
	return appBundle, nil
}

// writeMacIcns builds Contents/Resources/icon.icns from a source PNG using sips
// (to render the iconset sizes) and iconutil (to pack them). Both ship with
// macOS. Best-effort: on any failure it leaves no icns and the app uses the
// generic icon.
func writeMacIcns(appBundle, srcPNG string) {
	iconutil, err := exec.LookPath("iconutil")
	if err != nil {
		return
	}
	sips, err := exec.LookPath("sips")
	if err != nil {
		return
	}
	tmp, err := os.MkdirTemp("", "iconset")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // temp cleanup, nothing to act on
	iconset := filepath.Join(tmp, "icon.iconset")
	if err := os.MkdirAll(iconset, 0o755); err != nil {
		return
	}
	// The five base sizes plus their @2x variants — the standard iconset set.
	for _, s := range []int{16, 32, 128, 256, 512} {
		one := filepath.Join(iconset, fmt.Sprintf("icon_%dx%d.png", s, s))
		two := filepath.Join(iconset, fmt.Sprintf("icon_%dx%d@2x.png", s, s))
		sz, dz := strconv.Itoa(s), strconv.Itoa(s*2)
		if err := exec.Command(sips, "-z", sz, sz, srcPNG, "--out", one).Run(); err != nil {
			return
		}
		if err := exec.Command(sips, "-z", dz, dz, srcPNG, "--out", two).Run(); err != nil {
			return
		}
	}
	resources := filepath.Join(appBundle, "Contents", "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return
	}
	_ = exec.Command(iconutil, "-c", "icns", iconset,
		"-o", filepath.Join(resources, "icon.icns")).Run()
}

// removeLauncher: nothing to remove beyond the bundle itself (handled by
// RemoveStagedInstall). Deregistering from Launch Services is unnecessary —
// removing the bundle invalidates the registration.
func (a AppSpec) removeLauncher(_ bool) error { return nil }

// appRoot returns the nearest ".app" ancestor of exePath, or exePath's directory
// if none (a flat staging that isn't a real bundle).
func appRoot(exePath string) string {
	dir := exePath
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(exePath) // no .app ancestor
		}
		if filepath.Ext(parent) == ".app" {
			return parent
		}
		dir = parent
	}
}
