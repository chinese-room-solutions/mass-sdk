package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/term"
	"github.com/stretchr/testify/require"
)

// Go↔C++ TUI parity goldens. The worker's C++ installer (mass-worker-llama-cpp)
// renders the same form/menu/modal layouts from an independent code base; these
// fixtures pin the Go rendering with styling forced off (pure layout, no SGR)
// so the C++ side can consume the SAME testdata files and diff its own output
// against them. Regenerate with: go test ./tui -run TestGolden -update
//
// Fixtures are normalized (trailing whitespace stripped per line, LF endings)
// so they survive editors and git autocrlf — normalize the same way on the C++
// side before diffing.

var updateGolden = flag.Bool("update", false, "rewrite the golden fixtures under testdata/")

// normalizeGolden strips per-line trailing whitespace and normalizes CRLF.
func normalizeGolden(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// assertGolden compares got with testdata/<name>, rewriting the fixture when
// the -update flag is set.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	got = normalizeGolden(got)
	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing fixture %s — run with -update to create it", path)
	require.Equal(t, normalizeGolden(string(want)), got, "rendering drifted from %s (run with -update if intended)", path)
}

// forcePlainCaps turns styling off for the test so the render is pure layout.
func forcePlainCaps(t *testing.T) {
	t.Helper()
	restore := term.ForceCaps(false, false)
	t.Cleanup(restore)
}

var goldenBannerArt = []string{
	` __  __    _    ____  ____ `,
	`|  \/  |  / \  / ___|/ ___|`,
	`| |\/| | / _ \ \___ \\___ \`,
	`|_|  |_|/_/ \_\|____/|____/`,
}

// TestGoldenForm renders the full form frame (banner → fields → actions →
// status, wrapped in the margin family) exactly as RunForm would on its snapped
// window. The first field is the cursor row (the "> " marker) and one label is
// longer than the label column, so the clip-with-"…" rule is pinned.
func TestGoldenForm(t *testing.T) {
	forcePlainCaps(t)

	st := &formState{spec: FormSpec{
		BannerArt: goldenBannerArt,
		Tag:       "[ mass | 0.0.0 ]",
		Hint:      "Arrow keys move · edit a field · then choose an action.",
		Fields: []Field{
			{Label: "Installation scope", Kind: FieldChoice, Choices: []string{"User", "System"}, Value: "User"},
			{Label: "Install directory", Kind: FieldPath, Value: "/home/u/.local/lib/mass"},
			{Label: "A label long enough to clip in the column", Kind: FieldText, Value: "value"},
			{Label: "Web UI listen address (host:port)", Kind: FieldText, Value: ":3455"},
		},
		Actions: []string{"Install", "Uninstall", "Exit"},
	}}
	st.fields = append([]Field(nil), st.spec.Fields...)

	// The exact RunForm entry sequence: measure the content box, snap the grid.
	st.contentW = formContentWidth(st)
	contentRows := formContentHeight(st)
	st.gridRows = contentRows + formMarginTop + formMarginBottom
	st.gridCols = st.contentW + formMarginLeft + formMarginRight

	var out strings.Builder
	orig := stdout
	stdout = &out
	defer func() { stdout = orig }()
	renderForm(st)

	assertGolden(t, "form_clipped_label.txt", out.String())
}

// TestGoldenMenu renders one menu block covering every row shape: a plain row,
// the selected row (the "> " marker), a choice row, the SELECTED choice row,
// and an over-long value that clips with "…".
func TestGoldenMenu(t *testing.T) {
	forcePlainCaps(t)

	layout := MenuLayout{Marker: 2, LabelCol: 28, Gap: 12, ValueCol: 30, MinValueCol: 16}
	geo := MenuGeometryFor(layout, 92)
	rows := []MenuRow{
		{Left: "Install directory", Right: "/home/u/.local/lib/mass"},
		{Left: "Data directory", Right: "/home/u/.local/share/mass", Style: MenuRowSelected},
		{Left: "Installation scope", Right: "User", IsChoice: true},
		{Left: "GPU backend", Right: "CUDA", IsChoice: true, Style: MenuRowSelected},
		{Left: "Model path", Right: "/models/a/very/long/path/that/exceeds/the/value/column/width.gguf"},
	}

	var out strings.Builder
	for _, r := range rows {
		out.WriteString(RenderMenuRow(r, layout, geo))
		out.WriteString("\n")
	}
	assertGolden(t, "menu_selected_choice.txt", out.String())
}

// TestGoldenModal renders a confirm modal whose prose message is split into
// sentences and word-wrapped to the margin, pinning the wrap+margin rules the
// worker's C++ modal mirrors.
func TestGoldenModal(t *testing.T) {
	forcePlainCaps(t)

	spec := ModalSpec{
		BannerArt: goldenBannerArt,
		Tag:       "[ mass | 0.0.0 ]",
		Lines: proseLines([]string{
			"Uninstalling removes the staged binaries and the launcher from this machine. " +
				"The data directory and everything in it is kept, so a later reinstall picks your settings back up. " +
				"Continue?",
		}),
		Buttons:  [2]string{"Yes", "No"},
		Selected: 1,
		Footer:   "←/→ or Tab move · Enter confirm · y / n · Esc cancels",
	}
	assertGolden(t, "modal_wrapped_message.txt", composeModal(spec, 1, 80))
}
