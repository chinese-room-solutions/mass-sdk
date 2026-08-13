package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/fsutil"
	"github.com/chinese-room-solutions/mass-sdk/selfextract"
)

// ErrStage marks a staging failure; wraps keep the underlying cause in the
// chain (multi-%w), so errors.Is works against both this sentinel and the
// original error (e.g. os.ErrPermission).
var ErrStage = errors.New("install: staging failed")

// Stage copies the app binary plus every sibling shared library from the
// running binary's directory into installDir, creating it if needed (files
// overwrite, so a reinstall/upgrade refreshes in place). Returns the absolute
// path of the staged executable.
//
// "Sibling shared library" = *.dll on Windows, *.so/*.so.N on Linux, *.dylib on
// macOS: the app's runtime deps sit beside the exe, so copying every sibling
// shared lib makes the install self-contained without a manifest that drifts.
//
// A no-op self-copy (installDir already IS the binary's directory — an in-place
// reinstall) is detected and skipped rather than erroring. Requires write access
// to installDir (elevation for the default machine-wide locations).
//
// Two provisioning modes are chosen automatically:
//   - packaged installer (this setup binary has an appended selfextract payload):
//     extract the bundled app binary + assets into installDir.
//   - unpackaged run from a build/dist tree (no payload): copy the app binary +
//     its sibling shared libraries from beside this setup binary.
//
// progress, when set, is forwarded to the payload extraction (called per file)
// so a setup flow can render a bar; nil, or the copy-siblings mode, reports
// nothing.
func (a AppSpec) Stage(installDir string, progress selfextract.ProgressFn) (string, error) {
	srcDir, err := CurrentExecutableDir()
	if err != nil {
		return "", fmt.Errorf("%w: locating own path: %w", ErrStage, err)
	}
	dstExe := a.StagedExePath(installDir)
	// stageDir is where the files physically land. It equals installDir except
	// for a macOS .app bundle, where the binary + libs go under Contents/MacOS
	// (StagedExePath already points there) and the bundle gets a Contents/
	// Info.plist below so Finder treats it as a launchable app.
	stageDir := filepath.Dir(dstExe)
	packaged := selfextract.SelfHasPayload()

	// In-place reinstall (src == dst) with no payload: nothing to copy. (A
	// packaged installer always extracts — it isn't running from the install dir.)
	if !packaged {
		if same, _ := SameDir(srcDir, stageDir); same {
			if _, err := os.Stat(dstExe); err == nil {
				return dstExe, nil
			}
		}
	}

	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return "", fmt.Errorf("%w: creating %s: %w", ErrStage, stageDir, err)
	}

	if packaged {
		if _, err := selfextract.ExtractSelf(stageDir, progress); err != nil {
			return "", fmt.Errorf("%w: %w", ErrStage, err)
		}
	} else if err := a.copySiblings(srcDir, stageDir); err != nil {
		return "", err
	}

	// Finish a macOS .app: write Contents/Info.plist so Finder/Launch Services
	// recognise the bundle (a bare binary under Contents/MacOS isn't launchable
	// without it). No-op on other OSes and non-bundle install dirs.
	if err := a.writeMacBundleMetadata(installDir); err != nil {
		return "", err
	}

	if _, err := os.Stat(dstExe); err != nil {
		return "", fmt.Errorf("%w: staged binary not found at %s after provisioning", ErrStage, dstExe)
	}
	return dstExe, nil
}

// copySiblings copies the app binary (by its fixed leaf name) plus every sibling
// shared library from srcDir into dstDir.
func (a AppSpec) copySiblings(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("%w: reading %s: %w", ErrStage, srcDir, err)
	}
	exeLeaf := a.ExeLeaf()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name != exeLeaf && !isSharedLibrary(name) {
			continue
		}
		if err := copyFile(filepath.Join(srcDir, name), filepath.Join(dstDir, name)); err != nil {
			return fmt.Errorf("%w: copying %s: %w", ErrStage, name, err)
		}
	}
	return nil
}

// RemoveStagedInstall deletes a previously-staged install directory and its
// contents. Returns the number of top-level entries removed.
//
// Safety: refuses to delete the directory holding THIS running executable (an
// uninstall launched from the installed copy would otherwise yank the binary out
// from under itself) — in that case it reports selfSkipped=true and removes
// nothing. A missing directory is not an error (already gone → 0). Requires write
// access (elevation for the default locations).
func RemoveStagedInstall(installDir string) (removed int, selfSkipped bool, err error) {
	if _, statErr := os.Stat(installDir); errors.Is(statErr, os.ErrNotExist) {
		return 0, false, nil // already gone
	}

	// Refuse to delete the directory we run from. Compare resolved paths so
	// symlinks / "." / trailing separators don't fool it.
	if ownDir, derr := CurrentExecutableDir(); derr == nil {
		if same, _ := SameDir(ownDir, installDir); same {
			return 0, true, nil
		}
	}

	entries, rerr := os.ReadDir(installDir)
	if rerr != nil {
		return 0, false, fmt.Errorf("install: reading %s: %w", installDir, rerr)
	}
	if err := os.RemoveAll(installDir); err != nil {
		return 0, false, fmt.Errorf("install: removing %s: %w", installDir, err)
	}
	return len(entries), false, nil
}

// isSharedLibrary reports whether name is a shared library that should be staged
// next to the binary, per the platform's convention.
func isSharedLibrary(name string) bool {
	switch runtime.GOOS {
	case "windows":
		return strings.EqualFold(filepath.Ext(name), ".dll")
	case "darwin":
		return filepath.Ext(name) == ".dylib"
	default:
		// .so or a versioned .so.N — match any name containing ".so".
		return strings.Contains(name, ".so")
	}
}

// SameDir reports whether two paths refer to the same directory, comparing
// resolved absolute paths (so symlinks / "." / trailing separators agree). The
// setup flow uses it to reject a data dir equal to the install dir (uninstall
// deletes the install dir wholesale).
func SameDir(a, b string) (bool, error) {
	ra, err := resolve(a)
	if err != nil {
		return false, err
	}
	rb, err := resolve(b)
	if err != nil {
		return false, err
	}
	return ra == rb, nil
}

func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		abs = r
	}
	return filepath.Clean(abs), nil
}

// copyFile copies src to dst, preserving the source's mode bits so an
// executable stays executable. An existing dst is replaced by rename rather
// than truncated in place — an upgrade re-runs the installer over binaries that
// may still be running, and truncating those breaks on every platform.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only source, close error is moot
	info, err := in.Stat()
	if err != nil {
		return err
	}
	_, err = fsutil.WriteAtomic(dst, in, info.Mode(), nil)
	return err
}
