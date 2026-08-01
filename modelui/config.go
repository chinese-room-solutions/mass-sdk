// Package modelui renders the llama model-config form and decodes its
// Datastar signals: RenderModelConfigForm binds the Signal* constants,
// MergeSignalsIntoConfig reads the same names back into a ModelConfigData.
// It builds on uikit for the generic page kit.
package modelui

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/uikit"
)

// ModelConfigData holds model configuration values for rendering the config form.
type ModelConfigData struct {
	Path          string
	ContextSize   int32
	Threads       int32
	GpuLayers     int32
	MaxConcurrent int32
	MainGPU       string
	TensorSplit   string
}

// Model-config Datastar signal names — the single definition shared by
// RenderModelConfigForm (data-bind attributes) and MergeSignalsIntoConfig
// (decoding), so the two sides can never drift on spelling or casing.
const (
	SignalModelPath     = "modelPath"
	SignalContextSize   = "contextSize"
	SignalThreads       = "threads"
	SignalGpuLayers     = "gpuLayers"
	SignalMaxConcurrent = "maxConcurrent"
	SignalMainGpu       = "mainGpu"
	SignalTensorSplit   = "tensorSplit"
)

// RenderModelConfigForm returns an interactive HTML form for model configuration.
// moduleName is used to construct the save action URL.
// The form uses Datastar data-bind with the Signal* constants above.
func RenderModelConfigForm(moduleName string, cfg ModelConfigData) string {
	esc := html.EscapeString
	var b strings.Builder

	b.WriteString(`<div class="space-y-3">`)

	// Embedding model path with Browse + Find on HuggingFace buttons aligned to the input.
	b.WriteString(`<div>`)
	b.WriteString(`<div class="text-sm mb-1" style="color:var(--sl-input-label-color,inherit)">Embedding Model Path</div>`)
	b.WriteString(`<div class="flex items-end gap-2">`)
	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" class="flex-1" value="%s" size="small" data-bind="%s" placeholder="/path/to/model.gguf"></sl-input>`,
		esc(cfg.Path), SignalModelPath)
	fmt.Fprintf(&b, `<sl-button size="small" variant="default" style="flex-shrink:0;margin-bottom:1px" onclick="window.__massBrowse('%s', '.gguf')"><sl-icon slot="prefix" name="folder2-open"></sl-icon>Browse</sl-button>`, SignalModelPath)
	fmt.Fprintf(&b, `<sl-button size="small" variant="default" style="flex-shrink:0;margin-bottom:1px" onclick="window.__massModelSelect('%s', 'Embedding')"><sl-icon slot="prefix" name="list-ul"></sl-icon>Select</sl-button>`, SignalModelPath)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="text-xs mt-1" style="color:var(--sl-input-help-text-color,#737373)">Path to GGUF model file</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="grid grid-cols-2 gap-3">`)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Context Size" value="%d" type="number" size="small" data-bind="%s"></sl-input>`,
		cfg.ContextSize, SignalContextSize)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Threads" value="%d" type="number" size="small" data-bind="%s"></sl-input>`,
		cfg.Threads, SignalThreads)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="GPU Layers" value="%d" type="number" size="small" data-bind="%s" help-text="0=all GPU, -1=CPU only"></sl-input>`,
		cfg.GpuLayers, SignalGpuLayers)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Max Concurrent" value="%d" type="number" size="small" data-bind="%s"></sl-input>`,
		cfg.MaxConcurrent, SignalMaxConcurrent)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Main GPU" value="%s" size="small" data-bind="%s" help-text="GPU device index for multi-GPU"></sl-input>`,
		esc(cfg.MainGPU), SignalMainGpu)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Tensor Split" value="%s" size="small" data-bind="%s" help-text="Multi-GPU ratio e.g. 0.7,0.3"></sl-input>`,
		esc(cfg.TensorSplit), SignalTensorSplit)

	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	return b.String()
}

// RenderConfigStatus returns an HTML fragment for the config save status area.
func RenderConfigStatus(msg string, isError bool) string {
	if msg == "" {
		return `<div id="pe-config-status"></div>`
	}
	variant := "success"
	if isError {
		variant = "danger"
	}
	return uikit.RenderAlert("pe-config-status", msg, variant, 3000)
}

// MergeSignalsIntoConfig overrides a ModelConfigData with values from
// MASS's gui-config signals, so the module UI shows persisted values.
//
// Decoding is presence-aware: a key absent from signals leaves the config
// field untouched (a request whose signals don't carry the model form must not
// zero Threads/GpuLayers). Present keys apply subject to per-field validity:
// Threads and GpuLayers apply verbatim (0 and -1 are meaningful), ContextSize
// and MaxConcurrent only when > 0, strings only when non-empty. Keys are the
// Signal* constants — the exact names RenderModelConfigForm binds.
func MergeSignalsIntoConfig(cfg ModelConfigData, signals json.RawMessage) ModelConfigData {
	var s map[string]any
	if err := json.Unmarshal(signals, &s); err != nil {
		return cfg
	}
	if v, ok := s[SignalModelPath].(string); ok && v != "" {
		cfg.Path = v
	}
	if raw, ok := s[SignalContextSize]; ok {
		if v := uikit.ToInt32(raw); v > 0 {
			cfg.ContextSize = v
		}
	}
	if raw, ok := s[SignalThreads]; ok {
		cfg.Threads = uikit.ToInt32(raw)
	}
	if raw, ok := s[SignalGpuLayers]; ok {
		cfg.GpuLayers = uikit.ToInt32(raw)
	}
	if raw, ok := s[SignalMaxConcurrent]; ok {
		if v := uikit.ToInt32(raw); v > 0 {
			cfg.MaxConcurrent = v
		}
	}
	if v, ok := s[SignalMainGpu].(string); ok && v != "" {
		cfg.MainGPU = v
	}
	if v, ok := s[SignalTensorSplit].(string); ok && v != "" {
		cfg.TensorSplit = v
	}
	return cfg
}
