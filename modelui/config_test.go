package modelui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			signals: `{"modelPath":"/new/path.gguf","contextSize":"4096","threads":"8","gpuLayers":"32","maxConcurrent":"2","mainGpu":"0","tensorSplit":"0.7,0.3"}`,
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
			name:    "empty signals keeps base untouched",
			signals: `{}`,
			want:    base,
		},
		{
			name:    "missing keys leave their fields untouched",
			signals: `{"contextSize":"4096"}`,
			want: ModelConfigData{
				Path:          "/default/model.gguf",
				ContextSize:   4096,
				Threads:       4,
				GpuLayers:     0,
				MaxConcurrent: 1,
			},
		},
		{
			name:    "present zero and negative apply to threads and gpuLayers",
			signals: `{"threads":0,"gpuLayers":-1}`,
			want: ModelConfigData{
				Path:          "/default/model.gguf",
				ContextSize:   2048,
				Threads:       0,
				GpuLayers:     -1,
				MaxConcurrent: 1,
			},
		},
		{
			name:    "invalid contextSize and maxConcurrent are ignored",
			signals: `{"contextSize":0,"maxConcurrent":0}`,
			want:    base,
		},
		{
			name:    "lowercase keys do not match the camelCase signals",
			signals: `{"modelpath":"/new/path.gguf","gpulayers":"32"}`,
			want:    base,
		},
		{
			name:    "invalid JSON keeps base",
			signals: `{invalid`,
			want:    base,
		},
		{
			name:    "float64 values from JSON numbers",
			signals: `{"contextSize":512,"threads":2,"gpuLayers":10,"maxConcurrent":3}`,
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

// The form renderer and the merge helper must agree on the signal names — the
// rendered data-bind attributes are exactly the Signal* constants the decoder
// reads.
func TestRenderModelConfigFormBindsSignalConstants(t *testing.T) {
	got := RenderModelConfigForm("llama", ModelConfigData{})
	for _, sig := range []string{
		SignalModelPath, SignalContextSize, SignalThreads, SignalGpuLayers,
		SignalMaxConcurrent, SignalMainGpu, SignalTensorSplit,
	} {
		require.Contains(t, got, `data-bind="`+sig+`"`)
	}
}
