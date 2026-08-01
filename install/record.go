package install

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/fsutil"
)

// Record is a breadcrumb the installer writes so a re-run can recover where the
// last install went. The install/data directories aren't stored anywhere else
// (the app's config lives under the data dir, but nothing points AT the data
// dir), so without this a reconfigure would always reset both prompts to the
// per-OS defaults even if the operator had redirected them.
//
// It lives at a fixed, always-discoverable, user-scoped location
// (RecordPath) — reachable on a re-run without elevation regardless of where the
// machine-wide install dir is. Tiny key=value file (same dependency-free format
// as an app .conf), written on install and removed on uninstall.
type Record struct {
	InstallDir string
	DataDir    string
	// CLIPath is what the PATH-exposure step created so uninstall can undo it
	// exactly: on Linux/macOS the absolute path of the symlink we placed in a bin
	// directory; on Windows the install directory we appended to the PATH env var.
	// Empty when no PATH entry was made (fell back to a hint, or skipped a foreign
	// file). Its meaning is OS-specific; only that OS's clipath code reads it.
	CLIPath string
}

// RecordPath is the absolute path of the app's install record, under the user
// config dir: <UserConfigDir>/<Name>/install.record. User-scoped so it's always
// writable/readable on a re-run without admin.
func (a AppSpec) RecordPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, a.Name, "install.record"), nil
}

// LoadRecord reads the install record, or (nil, nil) if none exists / is empty.
// A read error other than not-exist is returned.
func (a AppSpec) LoadRecord() (*Record, error) {
	path, err := a.RecordPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file, close error is moot

	var rec Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, rawVal, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		val := strings.TrimRight(rawVal, "\r")
		switch key {
		case "install_dir":
			rec.InstallDir = val
		case "data_dir":
			rec.DataDir = val
		case "cli_path":
			rec.CLIPath = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// A record with no directory is meaningless — treat as absent. (CLIPath alone
	// never occurs: it's only written alongside an install dir.)
	if rec.InstallDir == "" && rec.DataDir == "" {
		return nil, nil
	}
	return &rec, nil
}

// SaveRecord persists the install record, creating the directory if needed.
// Atomic (temp + rename via fsutil) so a crash mid-write can't leave a
// half-record. 0600 — it only holds local paths, but no reason to make it
// world-readable.
func (a AppSpec) SaveRecord(rec Record) error {
	path, err := a.RecordPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := "install_dir=" + rec.InstallDir + "\n" +
		"data_dir=" + rec.DataDir + "\n" +
		"cli_path=" + rec.CLIPath + "\n"
	return fsutil.WriteFileAtomic(path, []byte(body), 0o600)
}

// RemoveRecord deletes the install record (best-effort; missing is not an error).
func (a AppSpec) RemoveRecord() error {
	path, err := a.RecordPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
