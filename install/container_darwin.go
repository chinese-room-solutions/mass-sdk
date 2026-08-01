//go:build darwin

package install

import (
	"fmt"
	"os"
	"path/filepath"
)

// buildContainer wraps BinPath into a .app whose executable is a tiny launcher
// that hands the real binary to the dispatcher, which opens Terminal.app and runs
// the wizard there. Double-clicking the .app in Finder launches it.
func buildContainer(spec ContainerSpec) (string, error) {
	if spec.Name == "" || spec.ID == "" || spec.BinPath == "" {
		return "", fmt.Errorf("install: BuildContainer needs Name, ID and BinPath")
	}
	if _, err := os.Stat(spec.BinPath); err != nil {
		return "", fmt.Errorf("install: binary not found: %s", spec.BinPath)
	}
	if err := os.MkdirAll(spec.OutDir, 0o755); err != nil {
		return "", err
	}

	app := filepath.Join(spec.OutDir, spec.Name+".app")
	if err := os.RemoveAll(app); err != nil {
		return "", err
	}
	macOS := filepath.Join(app, "Contents", "MacOS")
	resources := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(macOS, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return "", err
	}

	if err := copyFile(spec.BinPath, filepath.Join(macOS, spec.ID)); err != nil {
		return "", err
	}
	if err := os.Chmod(filepath.Join(macOS, spec.ID), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(macOS, "dispatch.sh"),
		[]byte(spec.dispatcherScript()), 0o755); err != nil {
		return "", err
	}

	// The bundle executable is a launcher that resolves its own dir and hands the
	// real binary to the dispatcher.
	launch := fmt.Sprintf(`#!/bin/sh
HERE="$(cd "$(dirname "$0")" && pwd)"
exec "$HERE/dispatch.sh" "$HERE/%s" "$@"
`, spec.ID)
	if err := os.WriteFile(filepath.Join(macOS, "launch"), []byte(launch), 0o755); err != nil {
		return "", err
	}

	bundleID := spec.BundleID
	if bundleID == "" {
		bundleID = "com.chinese-room-solutions." + spec.ID
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>%s</string>
	<key>CFBundleIdentifier</key><string>%s</string>
	<key>CFBundleExecutable</key><string>launch</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleIconFile</key><string>icon</string>
</dict>
</plist>
`, spec.Name, bundleID)
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return "", err
	}

	// Best-effort icns from the PNG (sips + iconutil are macOS-only).
	if spec.IconPath != "" {
		writeMacIcns(app, spec.IconPath)
	}
	return app, nil
}
