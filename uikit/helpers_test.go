package uikit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToInt32(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want int32
	}{
		{"float64 positive", float64(42), 42},
		{"float64 zero", float64(0), 0},
		{"float64 negative", float64(-10), -10},
		{"float64 with fraction truncates", float64(3.9), 3},
		{"string positive", "123", 123},
		{"string zero", "0", 0},
		{"string negative", "-5", -5},
		{"string empty", "", 0},
		{"string non-numeric", "abc", 0},
		{"nil", nil, 0},
		{"int (unsupported type)", int(7), 0},
		{"bool (unsupported type)", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToInt32(tt.v)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFilenameToDlID(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"simple name", "model", "dl-model"},
		{"with extension", "model.gguf", "dl-model_gguf"},
		{"with dots and dashes", "my-model.Q4_K_M.gguf", "dl-my-model_Q4_K_M_gguf"},
		{"spaces replaced", "my model file", "dl-my_model_file"},
		{"empty string", "", "dl-"},
		{"only special chars", "!@#$%", "dl-_____"},
		{"mixed alphanumeric", "abc123-XYZ", "dl-abc123-XYZ"},
		{"path separators", "path/to/file", "dl-path_to_file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilenameToDlID(tt.filename)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRenderAlert(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		msg      string
		variant  string
		duration int
		checks   []string // substrings that must appear
	}{
		{
			name:    "empty message returns empty div",
			id:      "alert-1",
			msg:     "",
			variant: "success",
			checks:  []string{`<div id="alert-1"></div>`},
		},
		{
			name:    "basic alert without duration",
			id:      "alert-2",
			msg:     "Saved!",
			variant: "success",
			checks:  []string{`id="alert-2"`, `variant="success"`, `open`, `Saved!`},
		},
		{
			name:     "alert with duration",
			id:       "alert-3",
			msg:      "Error occurred",
			variant:  "danger",
			duration: 5000,
			checks:   []string{`variant="danger"`, `duration="5000"`, `Error occurred`},
		},
		{
			name:    "html-escapes id and message",
			id:      `a"b`,
			msg:     `<script>alert("xss")</script>`,
			variant: "warning",
			checks:  []string{`a&#34;b`, `&lt;script&gt;`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderAlert(tt.id, tt.msg, tt.variant, tt.duration)
			for _, sub := range tt.checks {
				require.True(t, strings.Contains(got, sub),
					"expected %q to contain %q", got, sub)
			}
		})
	}
}

func TestRenderAlert_NoDurationAttr(t *testing.T) {
	got := RenderAlert("x", "msg", "info", 0)
	require.NotContains(t, got, "duration")
}

func TestRenderConfigStatus(t *testing.T) {
	tests := []struct {
		name    string
		msg     string
		isError bool
		checks  []string
	}{
		{
			name:   "empty message",
			msg:    "",
			checks: []string{`<div id="pe-config-status"></div>`},
		},
		{
			name:   "success message",
			msg:    "Config saved",
			checks: []string{`variant="success"`, `duration="3000"`, `Config saved`},
		},
		{
			name:    "error message",
			msg:     "Failed",
			isError: true,
			checks:  []string{`variant="danger"`, `Failed`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderConfigStatus(tt.msg, tt.isError)
			for _, sub := range tt.checks {
				require.True(t, strings.Contains(got, sub),
					"expected %q to contain %q", got, sub)
			}
		})
	}
}

func TestMergeSignalsIntoConfig(t *testing.T) {
	base := ModelConfigData{
		Path:          "/default/model.gguf",
		ContextSize:   2048,
		Threads:       4,
		GpuLayers:     0,
		MaxConcurrent: 1,
		MainGPU:       "",
		TensorSplit:   "",
	}

	tests := []struct {
		name    string
		signals string
		want    ModelConfigData
	}{
		{
			name:    "overrides all fields",
			signals: `{"modelpath":"/new/path.gguf","contextsize":"4096","threads":"8","gpulayers":"32","maxconcurrent":"2","maingpu":"0","tensorsplit":"0.7,0.3"}`,
			want: ModelConfigData{
				Path:          "/new/path.gguf",
				ContextSize:   4096,
				Threads:       8,
				GpuLayers:     32,
				MaxConcurrent: 2,
				MainGPU:       "0",
				TensorSplit:   "0.7,0.3",
			},
		},
		{
			name:    "empty signals keeps base",
			signals: `{}`,
			want: ModelConfigData{
				Path:          "/default/model.gguf",
				ContextSize:   2048,
				Threads:       0,
				GpuLayers:     0,
				MaxConcurrent: 1,
			},
		},
		{
			name:    "invalid JSON keeps base",
			signals: `{invalid`,
			want:    base,
		},
		{
			name:    "float64 values from JSON numbers",
			signals: `{"contextsize":512,"threads":2,"gpulayers":10,"maxconcurrent":3}`,
			want: ModelConfigData{
				Path:          "/default/model.gguf",
				ContextSize:   512,
				Threads:       2,
				GpuLayers:     10,
				MaxConcurrent: 3,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeSignalsIntoConfig(base, []byte(tt.signals))
			require.Equal(t, tt.want, got)
		})
	}
}
