package uikit

import (
	"fmt"
	"html"
	"strings"
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

// RenderModelConfigForm returns an interactive HTML form for model configuration.
// moduleName is used to construct the save action URL.
// The form uses Datastar data-bind with standard signal names:
// modelPath, contextSize, threads, gpuLayers, maxConcurrent, mainGpu, tensorSplit.
func RenderModelConfigForm(moduleName string, cfg ModelConfigData) string {
	esc := html.EscapeString
	var b strings.Builder

	b.WriteString(`<div class="space-y-3">`)

	// Embedding model path with Browse + Find on HuggingFace buttons aligned to the input.
	b.WriteString(`<div>`)
	b.WriteString(`<div class="text-sm mb-1" style="color:var(--sl-input-label-color,inherit)">Embedding Model Path</div>`)
	b.WriteString(`<div class="flex items-end gap-2">`)
	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" class="flex-1" value="%s" size="small" data-bind="modelPath" placeholder="/path/to/model.gguf"></sl-input>`,
		esc(cfg.Path))
	b.WriteString(`<sl-button size="small" variant="default" style="flex-shrink:0;margin-bottom:1px" onclick="window.__massBrowse('modelPath', '.gguf')"><sl-icon slot="prefix" name="folder2-open"></sl-icon>Browse</sl-button>`)
	b.WriteString(`<sl-button size="small" variant="default" style="flex-shrink:0;margin-bottom:1px" onclick="window.__massModelSelect('modelPath', 'Embedding')"><sl-icon slot="prefix" name="list-ul"></sl-icon>Select</sl-button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="text-xs mt-1" style="color:var(--sl-input-help-text-color,#737373)">Path to GGUF model file</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="grid grid-cols-2 gap-3">`)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Context Size" value="%d" type="number" size="small" data-bind="contextSize"></sl-input>`,
		cfg.ContextSize)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Threads" value="%d" type="number" size="small" data-bind="threads"></sl-input>`,
		cfg.Threads)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="GPU Layers" value="%d" type="number" size="small" data-bind="gpuLayers" help-text="0=all GPU, -1=CPU only"></sl-input>`,
		cfg.GpuLayers)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Max Concurrent" value="%d" type="number" size="small" data-bind="maxConcurrent"></sl-input>`,
		cfg.MaxConcurrent)

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Main GPU" value="%s" size="small" data-bind="mainGpu" help-text="GPU device index for multi-GPU"></sl-input>`,
		esc(cfg.MainGPU))

	fmt.Fprintf(&b,
		`<sl-input autocomplete="off" label="Tensor Split" value="%s" size="small" data-bind="tensorSplit" help-text="Multi-GPU ratio e.g. 0.7,0.3"></sl-input>`,
		esc(cfg.TensorSplit))

	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	return b.String()
}
