package tui

import (
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/term"
	"github.com/stretchr/testify/require"
)

// Cycling a choice field must fire OnFieldEdited (like an inline edit does), so a
// dependent field — e.g. install dirs that follow an install scope — gets
// re-seeded. Regression: the hook only fired on Enter (text edits), so cycling
// the scope left the paths stale.
func TestCycleChoiceFiresOnFieldEdited(t *testing.T) {
	calls := 0
	st := &formState{
		spec: FormSpec{
			Fields: []Field{
				{Label: "Scope", Kind: FieldChoice, Choices: []string{"User", "System"}, Value: "User"},
				{Label: "Dir", Kind: FieldPath, Value: "/user/dir"},
			},
			OnFieldEdited: func(idx int, fields []Field) []Field {
				calls++
				require.Equal(t, 0, idx) // the scope field
				next := append([]Field(nil), fields...)
				next[1].Value = "/system/dir" // re-seed the dependent field
				return next
			},
		},
	}
	st.fields = append([]Field(nil), st.spec.Fields...)
	st.cursor = 0

	// Cycle the scope right: User → System, which must re-seed Dir.
	done, _ := handleFieldKey(nil, st, Key{Type: KeyRight})
	require.False(t, done)
	require.Equal(t, 1, calls)
	require.Equal(t, "System", st.fields[0].Value)
	require.Equal(t, "/system/dir", st.fields[1].Value)
}

// A field entering the form seeded by Value alone (the natural caller shape)
// must start cycling from that value.
func TestNormalizeChoice(t *testing.T) {
	tests := []struct {
		name      string
		field     Field
		wantIdx   int
		wantValue string
	}{
		{
			name:      "value drives the index",
			field:     Field{Kind: FieldChoice, Choices: []string{"User", "System"}, Value: "System"},
			wantIdx:   1,
			wantValue: "System",
		},
		{
			name:      "unknown value snaps to the indexed choice",
			field:     Field{Kind: FieldChoice, Choices: []string{"User", "System"}, Value: "Weird", ChoiceIndex: 1},
			wantIdx:   1,
			wantValue: "System",
		},
		{
			name:      "unknown value and out-of-range index snap to the first choice",
			field:     Field{Kind: FieldChoice, Choices: []string{"User", "System"}, Value: "", ChoiceIndex: 5},
			wantIdx:   0,
			wantValue: "User",
		},
		{
			name:      "non-choice field untouched",
			field:     Field{Kind: FieldText, Value: "x", ChoiceIndex: 3},
			wantIdx:   3,
			wantValue: "x",
		},
		{
			name:      "empty choices untouched",
			field:     Field{Kind: FieldChoice, Value: "x"},
			wantIdx:   0,
			wantValue: "x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.field
			normalizeChoice(&f)
			require.Equal(t, tt.wantIdx, f.ChoiceIndex)
			require.Equal(t, tt.wantValue, f.Value)
		})
	}
}

// One ←/→ press must change a choice field even when the caller's OnFieldEdited
// re-seed returns fields synced by Value only. Regression: the installer rebuilt
// its fields on every scope flip with ChoiceIndex left at 0, so the first press
// after flipping to System re-landed on System — changing back to User took two
// presses.
func TestChoiceCycleSinglePressAfterReseed(t *testing.T) {
	scopeField := func(value string) Field {
		return Field{Label: "Scope", Kind: FieldChoice, Choices: []string{"User", "System"}, Value: value}
	}
	st := &formState{
		spec: FormSpec{
			Fields: []Field{scopeField("User")},
			// Rebuild the way installers do: the value carries over, the index is
			// not set.
			OnFieldEdited: func(_ int, fields []Field) []Field {
				return []Field{scopeField(fields[0].Value)}
			},
		},
	}
	st.fields = append([]Field(nil), st.spec.Fields...)
	normalizeChoices(st.fields)

	// User → System in one press, then System → User in ONE press through the
	// re-seeded fields.
	handleFieldKey(nil, st, Key{Type: KeyRight})
	require.Equal(t, "System", st.fields[0].Value)
	handleFieldKey(nil, st, Key{Type: KeyRight})
	require.Equal(t, "User", st.fields[0].Value)
}

// A single-choice field can't change value, so cycling it must NOT fire the hook.
func TestCycleChoiceNoChangeNoNotify(t *testing.T) {
	calls := 0
	st := &formState{
		spec: FormSpec{
			Fields: []Field{{Label: "Only", Kind: FieldChoice, Choices: []string{"User"}, Value: "User"}},
			OnFieldEdited: func(int, []Field) []Field {
				calls++
				return nil
			},
		},
	}
	st.fields = append([]Field(nil), st.spec.Fields...)
	st.cursor = 0
	handleFieldKey(nil, st, Key{Type: KeyRight})
	require.Equal(t, 0, calls)
}

func sampleFormState() *formState {
	return &formState{spec: FormSpec{
		BannerArt: []string{
			` __  __    _    ____  ____ `,
			`|  \/  |  / \  / ___|/ ___|`,
			`| |\/| | / _ \ \___ \\___ \`,
			`|_|  |_|/_/ \_\|____/|____/`,
		},
		Tag:  "[ mass | dev ]",
		Hint: "Arrow keys move · edit a field · then choose an action.",
		Fields: []Field{
			{Label: "Installation scope", Kind: FieldChoice, Choices: []string{"User", "System"}, Value: "User"},
			{Label: "Install directory", Kind: FieldPath, Value: "/home/u/.local/lib/mass"},
			{Label: "Data directory", Kind: FieldPath, Value: "/home/u/.local/share/mass"},
			{Label: "Web UI listen address", Kind: FieldText, Value: ":3455"},
		},
		Actions: []string{"Install", "Uninstall", "Exit"},
	}}
}

// The 4-field wizard's content must fit comfortably under the ~21-row window the
// launcher opens, with room to spare for the margin.
func TestFormContentHeightFits(t *testing.T) {
	st := sampleFormState()
	st.fields = st.spec.Fields
	st.contentW = formContentWidth(st)

	if content := formContentHeight(st); content > 19 {
		t.Fatalf("4-field content height %d unexpectedly tall (window opens ~21)", content)
	}
}

// The form wraps its content box in the margin family (one per side): the snapped
// window is content + top/bottom + left/right margins, and frameWithMargin emits
// exactly those borders with no extra incidental blanks.
func TestFrameMargins(t *testing.T) {
	st := sampleFormState()
	st.fields = st.spec.Fields
	st.contentW = formContentWidth(st)
	contentRows := formContentHeight(st)

	winRows := contentRows + formMarginTop + formMarginBottom
	body := composeBody(st, st.contentW)
	frame := frameWithMargin(body, winRows)

	lines := strings.Split(frame, "\n")
	require.Len(t, lines, winRows, "frame should fill the window height exactly")

	// Top margin: exactly formMarginTop blank rows; bottom: exactly formMarginBottom.
	for i := range formMarginTop {
		require.Empty(t, lines[i], "row %d should be a top-margin blank", i)
	}
	for i := range formMarginBottom {
		require.Empty(t, lines[winRows-1-i], "row %d should be a bottom-margin blank", winRows-1-i)
	}
	require.NotEmpty(t, strings.TrimSpace(lines[formMarginTop]),
		"the row just below the top margin must carry content (no extra blank)")
	require.NotEmpty(t, strings.TrimSpace(lines[winRows-1-formMarginBottom]),
		"the row just above the bottom margin must carry content (no extra blank)")

	require.Equal(t, formMarginLeft, leftEdge(lines), "content box left edge should be at column formMarginLeft")
}

// Horizontal margin is fixed (the box is left-anchored at formMarginLeft on every
// platform), and the top margin never shrinks below formMarginTop. When the window
// fits the box snugly the top margin IS formMarginTop; when it's taller than the box
// (a host that ignored the resize / won't shrink) the box centers vertically,
// top-biased, so the surplus is split rather than flooding the bottom — the top pad
// only grows, never drops. The frame always fills the whole window (no unpainted
// bottom → no scrollbar).
func TestFrameMarginsStableAcrossWindowSize(t *testing.T) {
	st := sampleFormState()
	st.fields = st.spec.Fields
	st.contentW = formContentWidth(st)
	contentRows := formContentHeight(st)

	tight := contentRows + formMarginTop + formMarginBottom
	for _, winRows := range []int{tight, tight + 10, tight + 40} {
		lines := strings.Split(frameWithMargin(composeBody(st, st.contentW), winRows), "\n")
		require.Len(t, lines, winRows, "frame fills the window height for winRows=%d", winRows)

		// The top margin is at least formMarginTop and grows with surplus (centered,
		// top-biased) — never below it, so the top border never looks cramped.
		topPad := topBlankRows(lines)
		require.GreaterOrEqual(t, topPad, formMarginTop, "winRows=%d: top margin floor", winRows)
		if winRows == tight {
			require.Equal(t, formMarginTop, topPad, "snug window: top margin is exactly formMarginTop")
		} else {
			require.Greater(t, topPad, formMarginTop, "winRows=%d: over-tall window centers, top pad grows", winRows)
			// Top-biased: the top pad is the smaller half of the surplus, so content
			// sits a touch above the vertical middle (never below it).
			botPad := winRows - topPad - contentRows
			require.LessOrEqual(t, topPad, botPad, "winRows=%d: content sits at or above vertical center", winRows)
		}
		require.NotEmpty(t, strings.TrimSpace(lines[topPad]),
			"winRows=%d: content must start right after the top margin", winRows)
		// Left margin is always exactly formMarginLeft.
		require.Equal(t, formMarginLeft, leftEdge(lines), "winRows=%d: left margin", winRows)
	}
}

// A window narrower than the form's natural box (e.g. a host that ignores the CSI 8t
// self-resize and stays at its 80-col default) must not overrun its right edge: the
// wide single-line elements (the footer, the hint) word-wrap and the field grid
// shrinks, so every emitted row fits the window with the left margin intact.
// Regression: renderForm floored the compose width at the natural box, so a
// too-narrow window composed past its own right edge — the footer spilled and the
// terminal hard-wrapped it with no right margin.
func TestNarrowWindowNeverOverruns(t *testing.T) {
	forcePlainCaps(t) // pure layout: no SGR/page-fill to mismeasure
	st := sampleFormState()
	st.fields = append([]Field(nil), st.spec.Fields...)
	st.contentW = formContentWidth(st)

	natural := st.contentW + formMarginLeft + formMarginRight
	require.Greater(t, natural, 80, "test premise: the sample form's natural box is wider than an 80-col window")

	// The narrowest window the form can host without squeezing: the field grid's own
	// minimum block (LabelCol + Gap + MinValueCol — the grid can't shrink past this)
	// plus the two side margins. Below this the layout genuinely can't fit and the
	// decline gate should send the caller to the linear fallback; at or above it,
	// every row must fit. The real bug lived well inside this range (80 cols).
	l := st.spec.menuLayout()
	gridFloor := l.LabelCol + l.Gap + l.MinValueCol + formMarginLeft + formMarginRight
	for _, winCols := range []int{80, 72, gridFloor} {
		st.gridRows, st.gridCols = 0, 0 // no snapped grid → renderForm uses the live size
		origSize := termSizeFn
		termSizeFn = func() termSize { return termSize{rows: 24, cols: winCols} }

		var out strings.Builder
		orig := stdout
		stdout = &out
		renderForm(st)
		stdout = orig
		termSizeFn = origSize

		// OnPage pads every row out to winCols, so measure the emitted content per row
		// (strip the page-fill escapes) — no row's VISIBLE width may exceed the window.
		for _, row := range strings.Split(out.String(), "\n") {
			if w := term.VisibleWidth(row); w > winCols {
				t.Fatalf("winCols=%d: row overruns by %d cols: %q", winCols, w-winCols, row)
			}
		}
	}
}

// The field menu anchors the GUTTER's midpoint on the centering axis (cols/2),
// not the whole block — so the label↔value gap sits dead-centre whatever the
// labels or values hold, matching the worker's menu component. Every field row
// shares one indent (the menu moves as a unit) and that indent places the gutter's
// centre on cols/2. The indent is applied regardless of styling, so this holds in
// the test environment.
func TestFieldMenuGutterAnchored(t *testing.T) {
	st := sampleFormState()
	st.fields = st.spec.Fields

	const cols = 100
	layout := formMenuLayout
	geo := MenuGeometryFor(layout, cols)
	body := composeBody(st, cols)

	// Field rows are the lines carrying a label. The menu moves as a unit, so each
	// row's indent measured from its LABEL (past the cursor marker, which shifts the
	// selected row) is the same — the menu's left margin within the box.
	var labelIndents []int
	for _, line := range strings.Split(body, "\n") {
		for _, f := range st.fields {
			if f.Label != "" && strings.Contains(line, f.Label) {
				labelIndents = append(labelIndents, strings.Index(line, f.Label))
			}
		}
	}
	require.NotEmpty(t, labelIndents, "should find field rows")

	indent := labelIndents[0]
	for _, in := range labelIndents {
		require.Equal(t, indent, in, "every field row shares the menu indent (measured from the label)")
	}
	require.Greater(t, indent, 0, "a menu narrower than cols must be indented, not flush-left")

	// The label starts at geo.Indent + marker (the marker sits inside the label
	// column). The gutter's centre is geo.Indent + LabelCol + Gap/2, which must land
	// on cols/2.
	require.Equal(t, geo.Indent+formMarker, indent, "label starts one marker into the menu")
	require.Equal(t, cols/2, geo.Indent+layout.LabelCol+layout.Gap/2,
		"the gutter's midpoint must sit on cols/2")
}

// The gutter-midpoint anchor must hold for ANY form, whatever its labels or field
// count — the gap always lands on cols/2, and the value column never collapses
// below the floor. (Worker parity: the menu degrades instead of spilling.)
func TestFieldMenuGutterAnchored_AnyLabels(t *testing.T) {
	const cols = 100
	cases := []struct {
		name   string
		labels []string
	}{
		{"two short labels", []string{"Installation scope", "Install directory"}},
		{"many long labels", []string{
			"Installation scope", "Install directory", "Data directory", "Listen address (host:port)",
		}},
		{"single label", []string{"Folder"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := sampleFormState()
			st.fields = make([]Field, len(tc.labels))
			for i, l := range tc.labels {
				st.fields[i] = Field{Label: l, Kind: FieldText, Value: "x"}
			}
			layout := formMenuLayout
			geo := MenuGeometryFor(layout, cols)
			require.Equal(t, cols/2, geo.Indent+layout.LabelCol+layout.Gap/2,
				"gutter midpoint must sit on cols/2 for %q", tc.name)
			require.GreaterOrEqual(t, geo.ValueWidth, layout.MinValueCol,
				"value column must never collapse below its floor for %q", tc.name)
		})
	}
}

// The field block's horizontal position must NOT depend on the field values:
// switching a path from long to short (e.g. a scope change swapping install dirs)
// or editing a value must not re-center the block. The indent is anchored to the
// fixed value column, not the current widest value, so every value variant yields
// the same label indent — no element shift as the user moves through the form.
func TestFieldBlockIndentStableAcrossValueWidths(t *testing.T) {
	const cols = 100

	labelIndent := func(values []string) int {
		st := sampleFormState()
		st.fields = append([]Field(nil), st.spec.Fields...)
		for i, v := range values {
			st.fields[i].Value = v
		}
		body := composeBody(st, cols)
		for _, line := range strings.Split(body, "\n") {
			if i := strings.Index(line, st.fields[1].Label); i >= 0 {
				return i
			}
		}
		t.Fatal("install-directory row not found")
		return -1
	}

	// Same fields, very different value widths on the install-dir row.
	short := labelIndent([]string{"User", "/x", "/y", ":1"})
	long := labelIndent([]string{"System", "/a/very/long/machine-wide/install/path/grimoire", "/y", ":1"})
	require.Equal(t, short, long, "block indent must not shift when a value's width changes")
}

// browsePath's three outcomes: a pick applies the value and does NOT fall back to
// the editor; a user cancel leaves the value and does NOT fall back (the user
// pressed Enter to browse, not 'e'); only "no picker on this platform" falls back
// to inline editing.
func TestBrowsePathOutcomes(t *testing.T) {
	orig := pickFolderFn
	defer func() { pickFolderFn = orig }()

	t.Run("pick applies, no fallback", func(t *testing.T) {
		pickFolderFn = func(string) (string, bool, error) { return "/picked/dir", true, nil }
		edited := 0
		st := &formState{spec: FormSpec{
			Fields:        []Field{{Label: "Install directory", Kind: FieldPath, Value: "/old"}},
			OnFieldEdited: func(int, []Field) []Field { edited++; return nil },
		}}
		st.fields = append([]Field(nil), st.spec.Fields...)
		st.cursor = 0

		require.False(t, browsePath(st, &st.fields[0]), "a pick must not drop into the editor")
		require.Equal(t, "/picked/dir", st.fields[0].Value)
		require.Equal(t, 1, edited, "a changed value fires OnFieldEdited")
	})

	t.Run("cancel leaves value, no fallback", func(t *testing.T) {
		pickFolderFn = func(string) (string, bool, error) { return "", false, nil }
		st := &formState{spec: FormSpec{
			Fields: []Field{{Label: "Install directory", Kind: FieldPath, Value: "/old"}},
		}}
		st.fields = append([]Field(nil), st.spec.Fields...)
		st.cursor = 0

		require.False(t, browsePath(st, &st.fields[0]), "a cancel must not open the editor")
		require.Equal(t, "/old", st.fields[0].Value, "value unchanged on cancel")
	})

	t.Run("no picker falls back to inline edit", func(t *testing.T) {
		pickFolderFn = func(string) (string, bool, error) { return "", false, ErrNoPicker }
		st := &formState{spec: FormSpec{
			Fields: []Field{{Label: "Install directory", Kind: FieldPath, Value: "/old"}},
		}}
		st.fields = append([]Field(nil), st.spec.Fields...)
		st.cursor = 0

		require.True(t, browsePath(st, &st.fields[0]), "no picker → caller edits inline")
		require.Equal(t, "/old", st.fields[0].Value)
	})
}

// A path field's status hint advertises Enter=browse / e=edit; a non-path field
// keeps the edit/confirm hint.
func TestStatusHintPerFieldKind(t *testing.T) {
	st := sampleFormState()
	st.fields = st.spec.Fields

	st.cursor = 1 // "Install directory" — a FieldPath
	require.Contains(t, renderStatusLine(st, 80), "browse")

	st.cursor = 3 // "Web UI listen address" — a FieldText
	require.NotContains(t, renderStatusLine(st, 80), "browse")
}

// RunForm distinguishes "environment can't host the TUI" (Declined → linear
// fallback) from "user aborted" (Cancelled → exit). Both env paths — a terminal
// too small and raw mode refused — must come back Declined, not Cancelled.
func TestRunFormDeclinesWhenEnvCannotHost(t *testing.T) {
	origSize := termSizeFn
	t.Cleanup(func() { termSizeFn = origSize })

	t.Run("terminal too small", func(t *testing.T) {
		termSizeFn = func() termSize { return termSize{rows: 5, cols: 20} }
		res, err := RunForm(sampleFormState().spec)
		require.NoError(t, err)
		require.True(t, res.Declined)
		require.False(t, res.Cancelled)
	})

	t.Run("raw mode unavailable", func(t *testing.T) {
		termSizeFn = func() termSize { return termSize{rows: 40, cols: 120} }
		stubNoRawMode(t)
		res, err := RunForm(sampleFormState().spec)
		require.NoError(t, err)
		require.True(t, res.Declined)
		require.False(t, res.Cancelled)
	})
}

// A hard cancel (Ctrl-C/EOF) inside an inline edit aborts the form as
// Cancelled — the user gave up, the environment is fine.
func TestEditHardCancelIsCancelledNotDeclined(t *testing.T) {
	st := sampleFormState()
	st.fields = append([]Field(nil), st.spec.Fields...)
	st.cursor = 3 // "Web UI listen address" — a FieldText, Enter edits inline

	// A nil RawMode makes the edit's first ReadKey fail — the hard-cancel path.
	done, res := handleFieldKey(nil, st, Key{Type: KeyEnter})
	require.True(t, done)
	require.True(t, res.Cancelled)
	require.False(t, res.Declined)
}

// FormSpec.Layout overrides individual field-grid columns; zero fields keep the
// package defaults, so an existing spec (zero Layout) renders identically.
func TestFormSpecLayoutOverrides(t *testing.T) {
	t.Run("zero layout keeps the defaults", func(t *testing.T) {
		require.Equal(t, formMenuLayout, FormSpec{}.menuLayout())
	})

	t.Run("non-zero fields apply, the rest stay default", func(t *testing.T) {
		got := FormSpec{Layout: MenuLayout{LabelCol: 40}}.menuLayout()
		want := formMenuLayout
		want.LabelCol = 40
		require.Equal(t, want, got)
	})

	t.Run("a widened LabelCol unclips a long label", func(t *testing.T) {
		long := strings.Repeat("L", formMenuLayout.LabelCol+4) // clips under the default
		newState := func(layout MenuLayout) *formState {
			st := sampleFormState()
			st.spec.Layout = layout
			st.spec.Fields[0] = Field{Label: long, Kind: FieldText, Value: "v"}
			st.fields = st.spec.Fields
			return st
		}

		require.Contains(t, composeBody(newState(MenuLayout{}), 120), "…",
			"the long label must clip under the default LabelCol")
		require.Contains(t, composeBody(newState(MenuLayout{LabelCol: len(long) + formMarker}), 120), long,
			"widening LabelCol must render the label whole")
	})
}

// leftEdge is the content box's left column: the smallest leading-space count
// across the non-blank rows (the banner centers within the box, so individual
// rows may be further indented).
func leftEdge(lines []string) int {
	minIndent := -1
	for _, row := range lines {
		if strings.TrimSpace(row) == "" {
			continue
		}
		lead := len(row) - len(strings.TrimLeft(row, " "))
		if minIndent < 0 || lead < minIndent {
			minIndent = lead
		}
	}
	return minIndent
}

// topBlankRows counts the leading all-whitespace rows — the rendered top margin.
func topBlankRows(lines []string) int {
	n := 0
	for _, row := range lines {
		if strings.TrimSpace(row) != "" {
			break
		}
		n++
	}
	return n
}
