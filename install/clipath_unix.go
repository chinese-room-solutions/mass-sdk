//go:build linux || darwin

package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// linkOnPath symlinks the staged binary into a bin directory that's on PATH so
// the app is runnable by its bare name from any terminal.
//
// Bin dir by scope:
//   - user:   ~/.local/bin (created if missing; on PATH by the systemd/profile
//     convention). macOS user fallback also lands here.
//   - system: /usr/local/bin. macOS prefers this when writable/elevated.
//
// Ownership rules: an existing symlink we own (points into our install dir) is
// replaced; a foreign regular file (or foreign symlink) is never clobbered — we
// skip it and return a warning hint, leaving OnPath false.
func (a AppSpec) linkOnPath(stagedExe string, scope Scope) (CLIResult, error) {
	binDir, existedOnPath := a.cliBinDir(scope)
	if binDir == "" {
		return CLIResult{}, fmt.Errorf("install: cannot resolve a bin directory for scope %q", scope)
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return CLIResult{}, fmt.Errorf("install: creating %s: %w", binDir, err)
	}

	link := filepath.Join(binDir, a.ExeName)
	installDir := installDirOf(stagedExe)

	// If something already occupies the link path, only replace it when it's a
	// symlink we own (resolves into our install dir). Never clobber a foreign
	// regular file or a symlink pointing elsewhere.
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return CLIResult{Hint: fmt.Sprintf("%q already exists and is not ours; left it in place — %s is not on PATH.", link, a.ExeName)}, nil
		}
		if !symlinkOwnedBy(link, installDir) {
			return CLIResult{Hint: fmt.Sprintf("%q is a symlink we don't own; left it in place — %s is not on PATH.", link, a.ExeName)}, nil
		}
		if err := os.Remove(link); err != nil {
			return CLIResult{}, fmt.Errorf("install: replacing %s: %w", link, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return CLIResult{}, fmt.Errorf("install: inspecting %s: %w", link, err)
	}

	if err := os.Symlink(stagedExe, link); err != nil {
		return CLIResult{}, fmt.Errorf("install: linking %s -> %s: %w", link, stagedExe, err)
	}

	res := CLIResult{Created: link, OnPath: true}
	if !existedOnPath {
		res.Hint = fmt.Sprintf("Add %s to your PATH to run %s from any terminal.", binDir, a.ExeName)
	}
	return res, nil
}

// unlinkFromPath removes the symlink recorded in created, but only if it's still
// a symlink pointing into installDir — so we never delete a foreign regular file
// or a link the operator re-pointed elsewhere at the same path. A missing link is
// not an error.
func (a AppSpec) unlinkFromPath(created, installDir string, _ Scope) error {
	if created == "" {
		return nil
	}
	info, err := os.Lstat(created)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	// Only a symlink we own is ours to remove; a regular file at this path isn't.
	if info.Mode()&os.ModeSymlink == 0 || !symlinkOwnedBy(created, installDir) {
		return nil
	}
	if err := os.Remove(created); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// symlinkOwnedBy reports whether link is a symlink whose resolved target lives
// inside installDir — the test for "a link we created" vs. a foreign one.
func symlinkOwnedBy(link, installDir string) bool {
	target, err := os.Readlink(link)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	resTarget, err := resolve(target)
	if err != nil {
		return false
	}
	resInstall, err := resolve(installDir)
	if err != nil {
		return false
	}
	return resTarget == resInstall || strings.HasPrefix(resTarget, resInstall+string(filepath.Separator))
}
