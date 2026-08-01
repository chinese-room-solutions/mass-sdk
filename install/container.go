package install

import (
	_ "embed"
	"strconv"
	"strings"
)

// Containers wrap a console program into the host OS's single double-clickable
// artifact, so a non-technical user can launch a terminal wizard with one click:
//
//	Linux  -> <id>-<arch>.AppImage   (one executable file)
//	macOS  -> <Name>.app             (one Finder icon; a bundle dir)
//	other  -> not supported (Windows consoles get a terminal from the OS)
//
// A bare binary launched from a file manager has no controlling terminal, so a
// TUI wizard would EOF on input and the window would flash closed. The container
// entry point runs an embedded dispatcher that opens the user's terminal, sized
// to the wizard, and runs the binary inside it. This is the Go port of the
// worker's make-bundle.sh, shared so MASS / Grimoire / the worker converge on one
// implementation rather than copies of a shell script.

//go:embed dispatch.sh
var dispatchTemplate string

// ContainerSpec describes the double-clickable wrapper to build around BinPath.
type ContainerSpec struct {
	// Name is the human label shown to the user (the .app's display name), e.g.
	// "MASS Setup".
	Name string
	// ID is the short, file-safe slug used for the bundled binary name and the
	// .desktop / AppImage basename, e.g. "mass-setup".
	ID string
	// BinPath is the console binary to wrap (the staged installer/app exe).
	BinPath string
	// OutDir is where the artifact is written.
	OutDir string
	// IconPath is an optional PNG (Linux) used as the launcher icon. Empty uses a
	// transparent placeholder so the build still succeeds.
	IconPath string
	// BundleID is the reverse-DNS id for the macOS Info.plist; empty falls back
	// to "com.chinese-room-solutions.<ID>".
	BundleID string
	// Cols/Rows size the terminal window the dispatcher opens. Zero uses the
	// defaults (sized to a typical setup wizard).
	Cols, Rows int
}

// defaults sized to a setup form (banner + fields + actions + footer ≈ 24 rows;
// the footer hint is the widest line at ~77 cols). The width fits that line plus
// the form's symmetric side margins (tui.formMarginLeft/Right, 5 each) so it
// reads as a centered card rather than touching the window edge.
const (
	defaultCols = 88
	defaultRows = 26
)

// dispatcherScript returns the embedded dispatcher with the window size baked in.
func (s ContainerSpec) dispatcherScript() string {
	cols, rows := s.Cols, s.Rows
	if cols <= 0 {
		cols = defaultCols
	}
	if rows <= 0 {
		rows = defaultRows
	}
	out := strings.ReplaceAll(dispatchTemplate, "__COLS__", strconv.Itoa(cols))
	return strings.ReplaceAll(out, "__ROWS__", strconv.Itoa(rows))
}

// BuildContainer builds the OS-appropriate double-clickable wrapper and returns
// the path of the artifact written. Unsupported OSes return an error.
func BuildContainer(spec ContainerSpec) (string, error) {
	return buildContainer(spec)
}
