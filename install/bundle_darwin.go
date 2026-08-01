//go:build darwin

package install

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeMacBundleMetadata completes a macOS .app: it writes Contents/Info.plist
// (and references an icon if one was staged) so Finder/Launch Services treat the
// directory as a launchable application. Without the plist a bare executable
// under Contents/MacOS opens a Terminal window instead of running as a GUI app.
// A no-op when installDir isn't a .app bundle.
func (a AppSpec) writeMacBundleMetadata(installDir string) error {
	if filepath.Ext(installDir) != ".app" {
		return nil
	}
	contents := filepath.Join(installDir, "Contents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		return fmt.Errorf("%w: creating %s: %w", ErrStage, contents, err)
	}

	bundleID := a.BundleID
	if bundleID == "" {
		bundleID = "com." + a.Name
	}
	// CFBundleIconFile is referenced unconditionally; if Resources/<icon>.icns
	// is absent Finder just shows the generic app icon — harmless.
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>%s</string>
	<key>CFBundleDisplayName</key><string>%s</string>
	<key>CFBundleIdentifier</key><string>%s</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleExecutable</key><string>%s</string>
	<key>CFBundleIconFile</key><string>icon</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
`, a.DisplayName, a.DisplayName, bundleID, a.ExeLeaf())

	plistPath := filepath.Join(contents, "Info.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("%w: writing %s: %w", ErrStage, plistPath, err)
	}
	return nil
}
