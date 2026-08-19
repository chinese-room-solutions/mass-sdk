package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/chinese-room-solutions/mass-sdk/install"
)

// Refusal is an update the app won't perform because of the operator's
// situation rather than a fault — not installed by the installer, installed
// where this user can't write. Msg is the sentence to show them; a caller
// answering HTTP maps a Refusal to 409 rather than 500.
type Refusal struct{ Msg string }

func (r *Refusal) Error() string { return r.Msg }

// Refuse builds a Refusal.
func Refuse(format string, args ...any) *Refusal {
	return &Refusal{Msg: fmt.Sprintf(format, args...)}
}

// IsRefusal reports whether err is (or wraps) a Refusal.
func IsRefusal(err error) bool {
	var r *Refusal
	return errors.As(err, &r)
}

// stageLeaf is the subdirectory of StageDir a fetched installer lands in.
const stageLeaf = "update"

// Applier installs a release over the app's own install by re-running the app's
// setup binary: it downloads that release's setup asset, verifies it against the
// release's SHA256SUMS, and starts it detached against the recorded install dir
// with --relaunch. The setup waits for the app's processes to exit, stages the
// new build, and starts it again — so the caller's last act after a nil return
// is to shut itself down.
type Applier struct {
	// App is the installer identity whose record names where the app lives.
	App install.AppSpec
	// BaseURL is the release repository.
	BaseURL string
	// StageDir is the directory the download lands under — the app's own data
	// dir, not the system temp, so the download's provenance and lifetime belong
	// to the app (macOS treats quarantine and temp cleanup differently there).
	StageDir string
	// Stdio, when set, is handed the installer command before it starts, so an
	// app with a spawn log can point the detached child's output at it.
	Stdio func(*exec.Cmd)
	// LoadRecord overrides the install-record lookup; nil reads the real one.
	LoadRecord func() (*install.Record, error)
	// FetchSetup overrides the verified download; nil uses [FetchSetup].
	FetchSetup func(ctx context.Context, baseURL, tag, asset, destDir string) (string, error)
}

// Apply installs tag over this install and returns once the installer is
// running. Its refusals — no install record, an install dir this user can't
// rewrite — come back as [Refusal].
func (a Applier) Apply(ctx context.Context, tag string) error {
	rec, err := a.loadRecord()
	if err != nil {
		return fmt.Errorf("reading the install record: %w", err)
	}
	name := a.App.DisplayName
	if rec == nil || rec.InstallDir == "" {
		// No recorded install dir: a `go run`, a binary copied by hand. Guessing
		// one would overwrite something we don't own.
		return Refuse("this %s wasn't installed by the %s installer, so it can't update itself — "+
			"download the latest installer and run it", name, name)
	}
	if !writableDir(rec.InstallDir) {
		// A machine-wide install. This doesn't elevate; the installer does.
		return Refuse("this %s is installed system-wide, so updating it needs administrator rights — "+
			"download the latest installer and run it", name)
	}

	dir, err := a.stageDir()
	if err != nil {
		return err
	}
	setupPath, err := a.fetchSetup(ctx, tag, dir)
	if err != nil {
		return fmt.Errorf("downloading the %s %s installer: %w", name, tag, err)
	}
	return a.runDetached(setupPath, rec.InstallDir)
}

func (a Applier) loadRecord() (*install.Record, error) {
	if a.LoadRecord != nil {
		return a.LoadRecord()
	}
	return a.App.LoadRecord()
}

func (a Applier) fetchSetup(ctx context.Context, tag, destDir string) (string, error) {
	fetch := a.FetchSetup
	if fetch == nil {
		fetch = FetchSetup
	}
	return fetch(ctx, a.BaseURL, tag, SetupAsset(a.App), destDir)
}

// SetupAsset is the release asset this platform installs from — the naked
// "<name>-setup" binary for the running OS/arch, under the evergreen name every
// release publishes.
func SetupAsset(app install.AppSpec) string {
	name := app.Name + "-setup_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// stageDir returns an empty directory for the download. A previous update's
// leftovers are cleared first: the installer it holds has already run, and an
// interrupted download must not be mistaken for a good one.
func (a Applier) stageDir() (string, error) {
	if a.StageDir == "" {
		return "", errors.New("selfupdate: no directory is configured for the update download")
	}
	dir := filepath.Join(a.StageDir, stageLeaf)
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("clearing the previous update download: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("preparing the update download dir: %w", err)
	}
	return dir, nil
}

// writableDir reports whether this process can rewrite the contents of dir —
// "can we install over it without elevation?", answered by doing the smallest
// version of the write rather than by reading permission bits (which say
// nothing useful on Windows).
func writableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".update-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// SetupArgs is the non-interactive install the downloaded setup is asked to
// perform: exactly the recorded install, in the scope that dir implies, with
// the relaunch that brings the app back afterwards. Spelling the scope out
// matters — the setup's own default is per-scope, which would move a custom
// install directory.
func SetupArgs(installDir string) []string {
	scope := install.ScopeSystem
	if install.IsUserScoped(installDir) {
		scope = install.ScopeUser
	}
	return []string{"--install", "--install-dir", installDir, "--scope", string(scope), "--relaunch"}
}

// runDetached starts the downloaded installer as a child that outlives this
// process — it has to, since its whole job is to replace the binaries this
// process is running from.
func (a Applier) runDetached(setupPath, installDir string) error {
	cmd := exec.Command(setupPath, SetupArgs(installDir)...) //nolint:gosec // our own verified download.
	if a.Stdio != nil {
		a.Stdio(cmd)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching the %s installer: %w", a.App.DisplayName, err)
	}
	return cmd.Process.Release()
}
