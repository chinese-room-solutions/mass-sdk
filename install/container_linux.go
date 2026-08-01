//go:build linux

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// A 1x1 transparent PNG, used when no icon is supplied (AppImages need one).
var placeholderPNG = []byte{
	0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R',
	0, 0, 0, 1, 0, 0, 0, 1, 0x08, 0x06, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
	0, 0, 0, 0x0a, 'I', 'D', 'A', 'T', 'x', 0x9c, 'c', 0, 1, 0, 0, 0x05,
	0, 1, 0x0d, '\n', '-', 0xb4, 0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 'B', '`', 0x82,
}

// buildContainer wraps BinPath into an AppImage: an AppDir holding the binary,
// the dispatcher, an AppRun entry point, a .desktop, and an icon — packed by
// appimagetool. The result runs the wizard in a terminal on double-click.
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

	tmp, err := os.MkdirTemp("", "appdir")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // temp cleanup
	appDir := filepath.Join(tmp, spec.ID+".AppDir")
	binDir := filepath.Join(appDir, "usr", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}

	if err := copyFile(spec.BinPath, filepath.Join(binDir, spec.ID)); err != nil {
		return "", err
	}
	if err := os.Chmod(filepath.Join(binDir, spec.ID), 0o755); err != nil {
		return "", err
	}

	// AppRun runs on double-click; point the dispatcher at the bundled binary.
	appRun := fmt.Sprintf(`#!/bin/sh
HERE="${APPDIR:-$(dirname "$(readlink -f "$0")")}"
exec "$HERE/dispatch.sh" "$HERE/usr/bin/%s" "$@"
`, spec.ID)
	if err := os.WriteFile(filepath.Join(appDir, "AppRun"), []byte(appRun), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(appDir, "dispatch.sh"),
		[]byte(spec.dispatcherScript()), 0o755); err != nil {
		return "", err
	}

	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec=AppRun
Icon=%s
Categories=Utility;
Terminal=false
`, spec.Name, spec.ID)
	if err := os.WriteFile(filepath.Join(appDir, spec.ID+".desktop"), []byte(desktop), 0o644); err != nil {
		return "", err
	}

	iconDst := filepath.Join(appDir, spec.ID+".png")
	if spec.IconPath != "" {
		if err := copyFile(spec.IconPath, iconDst); err != nil {
			return "", err
		}
	} else if err := os.WriteFile(iconDst, placeholderPNG, 0o644); err != nil {
		return "", err
	}

	tool, err := appimagetool()
	if err != nil {
		return "", err
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64" // appimagetool's ARCH naming
	}
	out := filepath.Join(spec.OutDir, fmt.Sprintf("%s-%s.AppImage", spec.ID, arch))
	// --appimage-extract-and-run avoids needing FUSE on the build host / CI.
	cmd := exec.Command(tool, "--appimage-extract-and-run", appDir, out)
	cmd.Env = append(os.Environ(), "ARCH="+arch)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("install: appimagetool failed: %w", err)
	}
	return out, nil
}

// appimagetool returns the path to a cached appimagetool, downloading it on
// first use into the user's cache dir.
func appimagetool() (string, error) {
	if env := os.Getenv("MASS_APPIMAGETOOL"); env != "" {
		return env, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	tool := filepath.Join(cache, "mass-build", "appimagetool")
	if _, err := os.Stat(tool); err == nil {
		return tool, nil
	}
	if err := os.MkdirAll(filepath.Dir(tool), 0o755); err != nil {
		return "", err
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	url := fmt.Sprintf(
		"https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-%s.AppImage", arch)
	fmt.Fprintf(os.Stderr, "install: fetching appimagetool -> %s\n", tool)
	cmd := exec.Command("curl", "-fsSL", "-o", tool, url)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("install: downloading appimagetool: %w", err)
	}
	if err := os.Chmod(tool, 0o755); err != nil {
		return "", err
	}
	return tool, nil
}
