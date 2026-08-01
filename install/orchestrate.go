package install

import "github.com/chinese-room-solutions/mass-sdk/selfextract"

// This file ties the per-step primitives (Stage, CreateLauncher, SaveRecord)
// into one Install/Uninstall call. A setup flow that wants to draw the steps as
// they happen passes Hooks rather than driving the primitives itself; they stay
// public for a flow that needs a different order or a subset.

// Plan is the input to Install: where to put the app and how to launch it. The
// data dir is the app's user-scoped config/state location (the app owns its
// format; the installer only records it so a re-run can recover it).
type Plan struct {
	InstallDir string
	DataDir    string
	Icon       string // optional absolute icon path for the launcher
	PerUser    bool   // user-scoped install + launcher (no elevation)
	Hooks      Hooks  // optional progress reporting for a setup flow to render
}

// Step names one of Install's steps, in the order they run.
type Step string

const (
	StepStage    Step = "stage"    // the app binary + assets are in place
	StepLauncher Step = "launcher" // the OS launcher (Start Menu / .desktop / .app)
	StepPath     Step = "path"     // the binary is exposed on PATH
	StepRecord   Step = "record"   // the install record is written
)

// Hooks reports an install as it happens, so a setup flow can render a step list.
// This package never writes to the console: it reports, the caller draws. Both
// fields are optional and the zero Hooks is silent — what a scripted face passes.
type Hooks struct {
	// Progress, when set, receives the packaged extraction's per-file progress
	// during the staging step, for a progress bar. Not called in the copy-siblings
	// mode (an unpackaged run from a build tree has no file count).
	Progress selfextract.ProgressFn
	// Step, when set, is called exactly once per step Install reaches, in order,
	// with that step's error (nil on success). A best-effort step (launcher, PATH)
	// reports its failure and the install carries on; a Stage failure is the last
	// report, because nothing after it runs.
	Step func(Step, error)
}

// report forwards one step's outcome when a Step hook is set.
func (h Hooks) report(step Step, err error) {
	if h.Step != nil {
		h.Step(step, err)
	}
}

// Result reports what Install did, for the setup flow to show.
type Result struct {
	StagedExe    string
	LauncherPath string
	// CLI reports the PATH-exposure step: what was created, whether the binary is
	// now runnable by name, and any hint the setup flow should print (bin dir not
	// on PATH, or "reopen your terminal" on Windows).
	CLI CLIResult
}

// scopeOf maps the Plan's PerUser flag to a Scope for the scope-aware steps.
func scopeOf(perUser bool) Scope {
	if perUser {
		return ScopeUser
	}
	return ScopeSystem
}

// Install stages the app, creates its launcher, exposes it on PATH, and writes the
// install record, in that order, reporting each step to plan.Hooks.
//
// The returned error is a FAILED INSTALL: staging, or the record that a later
// re-run/uninstall needs. The launcher and PATH steps are best-effort — the binary
// is staged and runnable regardless — so their failure is reported to Hooks.Step
// and left out of the error, and a caller that treats any error as fatal can't
// abort over one. Without hooks they are still visible in the Result: a failed
// launcher leaves LauncherPath empty, and CLI carries the PATH outcome.
func (a AppSpec) Install(plan Plan) (Result, error) {
	staged, err := a.Stage(plan.InstallDir, plan.Hooks.Progress)
	plan.Hooks.report(StepStage, err)
	if err != nil {
		return Result{}, err
	}

	res := Result{StagedExe: staged}

	lnk, lerr := a.CreateLauncher(LauncherSpec{
		ExePath:  staged,
		IconPath: plan.Icon,
		PerUser:  plan.PerUser,
	})
	plan.Hooks.report(StepLauncher, lerr)
	res.LauncherPath = lnk

	// Expose the binary on PATH so it's runnable by name from a terminal. Like the
	// launcher this is best-effort: a failure is reported but doesn't abort the
	// install (the binary is staged and runnable by full path).
	cli, cerr := a.LinkOnPath(staged, scopeOf(plan.PerUser))
	plan.Hooks.report(StepPath, cerr)
	res.CLI = cli

	// Record where we put things (including the PATH entry) so a re-run/uninstall
	// can find and undo them exactly.
	rerr := a.SaveRecord(Record{
		InstallDir: plan.InstallDir,
		DataDir:    plan.DataDir,
		CLIPath:    cli.Created,
	})
	plan.Hooks.report(StepRecord, rerr)
	return res, rerr
}

// Uninstall removes the launcher, the staged install, and the install record.
// Returns selfSkipped=true when the install dir couldn't be removed because the
// uninstaller is running from inside it (the caller should advise removing it
// manually, or relaunch from elsewhere) — in that case the install record is
// KEPT, so the orphaned install dir keeps its breadcrumb for a later re-run to
// find. A launcher/record removal failure is returned but does not stop the
// staged-install removal.
func (a AppSpec) Uninstall(installDir string, perUser bool) (selfSkipped bool, err error) {
	// Recover the recorded PATH entry (symlink path / Windows PATH dir) before we
	// remove the record, so we can undo exactly what we created.
	var cliPath string
	if rec, lerr := a.LoadRecord(); lerr == nil && rec != nil {
		cliPath = rec.CLIPath
	}

	lerr := a.RemoveLauncher(perUser)
	cerr := a.UnlinkFromPath(cliPath, installDir, scopeOf(perUser))
	_, selfSkipped, serr := RemoveStagedInstall(installDir)
	var rerr error
	if !selfSkipped {
		rerr = a.RemoveRecord()
	}

	// Prefer reporting the most consequential failure: the staged removal.
	switch {
	case serr != nil:
		return selfSkipped, serr
	case lerr != nil:
		return selfSkipped, lerr
	case cerr != nil:
		return selfSkipped, cerr
	case rerr != nil:
		return selfSkipped, rerr
	}
	return selfSkipped, nil
}
