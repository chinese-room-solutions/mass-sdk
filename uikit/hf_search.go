package uikit

import (
	"fmt"
	"html"
	"strings"
)

// HFResultModel holds data for rendering a HuggingFace search result.
type HFResultModel struct {
	RepoID      string
	Description string
	Downloads   int64
	Likes       int64
	Params      int64  // Total parameter count (e.g. 30_000_000_000 for 30B)
	PipelineTag string // HF pipeline_tag, e.g. "text-generation", "feature-extraction"
	AvatarURL   string // Author/org avatar image URL (may be empty)
	Files       []HFResultFile
}

// HFResultFile holds data for a downloadable GGUF file.
type HFResultFile struct {
	Filename  string
	SizeBytes int64
}

// HFResultsOpts controls rendering behaviour of HF result helpers.
type HFResultsOpts struct {
	HasMore bool // Whether the server has more results beyond the models provided.
	// DownloadedFiles maps "repoID/filename" to true for files already on disk.
	// When set, downloaded variants show a checkmark instead of a Get button.
	DownloadedFiles map[string]bool
	// MoreURL overrides the Show More POST URL. When empty, the default
	// module action path /api/modules/{name}/ui/action/show_more_hf is used.
	MoreURL string
	// DownloadURL overrides the download POST URL. When empty, defaults to
	// /mass.v1.Mass/DownloadModel. The endpoint receives JSON {repo_id, filename}.
	DownloadURL string
}

// repoToTplID converts a repo ID to a safe HTML template ID.
func repoToTplID(repoID string) string {
	s := strings.ReplaceAll(repoID, "/", "--")
	s = strings.ReplaceAll(s, ".", "_")
	return "hf-tpl-" + s
}

// RenderHFResults renders the full search results container.
// All models are rendered as visible rows. The server owns pagination state;
// Show More posts to the configured MoreURL to append additional rows via SSE.
func RenderHFResults(moduleName string, models []HFResultModel, opts HFResultsOpts) string {
	if len(models) == 0 {
		return `<div id="pe-hf-results"><p class="text-neutral-400 text-sm">No models found.</p></div>`
	}

	var b strings.Builder
	b.WriteString(`<div id="pe-hf-results">`)

	// Overlay + JS for variant picker.
	b.WriteString(`<style>.hf-chev{transition:transform .15s ease}[data-open="1"] .hf-chev{transform:rotate(180deg)}</style>`)
	b.WriteString(`<div id="hf-panel-overlay" style="display:none;position:fixed;z-index:9999;min-width:340px;max-width:540px;max-height:calc(100vh - 16px);overflow-y:auto;" class="bg-neutral-800 border border-neutral-700 rounded-lg shadow-2xl"></div>`)
	// Resolve download URL: default to the DownloadModel RPC endpoint.
	downloadURL := opts.DownloadURL
	if downloadURL == "" {
		downloadURL = "/mass.v1.Mass/DownloadModel"
	}
	escapedDlURL := html.EscapeString(downloadURL)

	b.WriteString(`<script>(function(){` +
		`var ov=document.getElementById('hf-panel-overlay'),ar=null;` +
		`var dlURL='` + escapedDlURL + `';` +
		`function cl(){ov.style.display='none';ov.innerHTML='';if(ar){ar.dataset.open='';ar=null;}}` +
		`document.addEventListener('mousedown',function(e){if(ov.style.display!=='none'&&!ov.contains(e.target)&&!e.target.closest('[data-hf-row]'))cl();},true);` +
		`window.__hfOpen=function(r){var t=document.getElementById(r.dataset.tpl);if(!t)return;if(ar===r){cl();return;}cl();ar=r;r.dataset.open='1';ov.appendChild(t.content.cloneNode(true));ov.style.display='block';` +
		`var rc=r.getBoundingClientRect(),vw=window.innerWidth,vh=window.innerHeight,ow=Math.min(540,vw-16);` +
		`ov.style.width=ow+'px';ov.style.left=Math.max(8,Math.min(rc.left,vw-ow-8))+'px';` +
		`var oh=ov.offsetHeight||320;ov.style.top=((rc.bottom+oh+8<=vh)?(rc.bottom+4):Math.max(8,rc.top-oh-4))+'px';};` +
		`window.__hfClose=cl;` +
		`window.__hfDlID=function(f){return 'dl-'+f.replace(/[^a-zA-Z0-9\\-]/g,'_');};` +
		// Find elements by id in both live DOM and inside <template> elements.
		`function dlEls(id){` +
		`var els=Array.from(document.querySelectorAll('[id="'+id+'"]'));` +
		`document.querySelectorAll('template').forEach(function(t){` +
		`var e=t.content.querySelector('[id="'+id+'"]');if(e)els.push(e);` +
		`});return els;}` +
		`window.__hfDownload=function(repo,file){` +
		`var id=window.__hfDlID(file);` +
		`dlEls(id).forEach(function(el){` +
		`el.innerHTML='<div style="position:relative;min-width:4.5rem;height:1.75rem;border-radius:0.25rem;overflow:hidden;background:#334155">` +
		`<div class="hf-dl-bar" style="position:absolute;top:0;left:0;height:100%;width:0%;background:#2563eb;transition:width .3s"></div>` +
		`<span class="hf-dl-pct" style="position:relative;z-index:1;display:flex;align-items:center;justify-content:center;height:100%;font-size:0.75rem;font-weight:600;color:#fff">0%</span></div>';` +
		`});` +
		`fetch(dlURL,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({repo_id:repo,filename:file})});` +
		`};` +
		`window.__hfDlProgress=function(file,pct){` +
		`dlEls(window.__hfDlID(file)).forEach(function(el){` +
		`var bar=el.querySelector('.hf-dl-bar');if(bar)bar.style.width=pct+'%';` +
		`var txt=el.querySelector('.hf-dl-pct');if(txt)txt.textContent=pct+'%';` +
		`});};` +
		`window.__hfDlDone=function(file){` +
		`var ck='<sl-icon name="check-circle-fill" style="color:#22c55e;font-size:1.2rem"></sl-icon>';` +
		`dlEls(window.__hfDlID(file)).forEach(function(el){el.innerHTML=ck;});` +
		`};` +
		`window.__hfDlErr=function(file,msg){` +
		`var h='<sl-tooltip content="'+msg.replace(/"/g,'&amp;quot;')+'"><sl-icon name="exclamation-triangle-fill" style="color:#f87171;font-size:1.2rem"></sl-icon></sl-tooltip>';` +
		`dlEls(window.__hfDlID(file)).forEach(function(el){el.innerHTML=h;});` +
		`};` +
		// Cancel: restore the "Get" button from repo/file data attributes on the span.
		`window.__hfDlCancel=function(file){` +
		`dlEls(window.__hfDlID(file)).forEach(function(el){` +
		`var repo=el.dataset.repo||'';` +
		`el.innerHTML='<sl-button size="small" variant="primary" onclick="window.__hfDownload(\''+repo.replace(/'/g,"\\'")+'\',\''+file.replace(/'/g,"\\'")+'\')"><sl-icon slot="prefix" name="download"></sl-icon>Get</sl-button>';` +
		`});};` +
		`})();</script>`)

	// Model list — all rows visible.
	b.WriteString(`<div id="pe-hf-list" class="space-y-px">`)
	for _, m := range models {
		renderHFModelRow(&b, m, opts.DownloadedFiles)
	}
	b.WriteString(`</div>`)

	// Footer with optional Show More.
	b.WriteString(RenderHFFooter(moduleName, opts.HasMore, opts.MoreURL))

	b.WriteString(`</div>`) // pe-hf-results
	return b.String()
}

// RenderHFResultRows renders model rows only (no container), for append-mode
// SSE patches when the user clicks Show More.
func RenderHFResultRows(models []HFResultModel, downloadedFiles map[string]bool) string {
	var b strings.Builder
	for _, m := range models {
		renderHFModelRow(&b, m, downloadedFiles)
	}
	return b.String()
}

// RenderHFFooter renders the Show More footer area.
// If hasMore is false the footer is an empty placeholder.
// moreURL overrides the POST URL; when empty, the default module action path is used.
func RenderHFFooter(moduleName string, hasMore bool, moreURL string) string {
	if !hasMore {
		return `<div id="pe-hf-footer"></div>`
	}
	if moreURL == "" {
		moreURL = "/api/modules/" + html.EscapeString(moduleName) + "/ui/action/show_more_hf"
	}
	return fmt.Sprintf(`<div id="pe-hf-footer" class="mt-2 text-center">`+
		`<sl-button id="pe-hf-more-btn" size="small" variant="text" `+
		`data-on:click="this.disabled=true;this.innerHTML='<sl-spinner style=\'font-size:1rem;--track-width:2px\'></sl-spinner>';@post('%s')">`+
		`<sl-icon slot="prefix" name="chevron-down"></sl-icon>Show More</sl-button></div>`,
		html.EscapeString(moreURL))
}

// renderHFModelRow writes template + row HTML for a single model.
func renderHFModelRow(b *strings.Builder, m HFResultModel, downloadedFiles map[string]bool) {
	esc := html.EscapeString
	modelName := FormatModelName(m.RepoID)
	tplID := repoToTplID(m.RepoID)

	// Template for variant picker.
	fmt.Fprintf(b, `<template id="%s">`, esc(tplID))
	renderHFVariantPanel(b, m, modelName, downloadedFiles)
	b.WriteString(`</template>`)

	// Row.
	fmt.Fprintf(b,
		`<div data-hf-row="1" data-tpl="%s" class="flex items-center gap-3 px-3 py-2.5 rounded cursor-pointer hover:bg-neutral-700/50 transition-colors bg-neutral-800/60" onclick="window.__hfOpen(this)">`,
		esc(tplID))

	if m.AvatarURL != "" {
		fmt.Fprintf(b, `<img src="%s" alt="" class="w-8 h-8 rounded-full flex-shrink-0 object-cover bg-neutral-700">`, esc(m.AvatarURL))
	} else {
		b.WriteString(`<div class="w-8 h-8 rounded-full bg-neutral-700 flex items-center justify-center flex-shrink-0 text-neutral-400"><sl-icon name="cpu" style="font-size:1rem"></sl-icon></div>`)
	}

	b.WriteString(`<div class="flex-1 min-w-0">`)
	fmt.Fprintf(b, `<div class="text-sm font-medium text-neutral-100 truncate">%s`, esc(modelName))
	if m.Params > 0 {
		fmt.Fprintf(b, ` <span class="font-mono text-xs font-bold bg-neutral-900 text-neutral-200 rounded px-1.5 py-0.5 ml-1" style="vertical-align:1px">%s</span>`, esc(FormatParams(m.Params)))
	}
	if label, ok := PipelineTagMap[m.PipelineTag]; ok {
		fmt.Fprintf(b, ` <span class="font-mono text-xs font-bold bg-neutral-900 text-neutral-200 rounded px-1.5 py-0.5 ml-1" style="vertical-align:1px">%s</span>`, esc(label))
	}
	if VisionPipelineTags[m.PipelineTag] {
		b.WriteString(` <sl-icon name="eye" style="font-size:0.85rem;vertical-align:-2px;color:#9ca3af;margin-left:6px" title="Vision"></sl-icon>`)
	}
	b.WriteString(`</div>`)
	fmt.Fprintf(b, `<div class="text-xs text-neutral-500 truncate">%s · %s ↓ · %s ♥</div>`,
		esc(m.RepoID), FormatCount(m.Downloads), FormatCount(m.Likes))
	b.WriteString(`</div>`)

	fileCount := len(m.Files)
	vl := fmt.Sprintf("%d variant", fileCount)
	if fileCount != 1 {
		vl += "s"
	}
	b.WriteString(`<div class="flex items-center flex-shrink-0">`)
	fmt.Fprintf(b, `<span class="text-xs bg-neutral-700 text-neutral-300 rounded px-1.5 py-0.5">%s</span>`, esc(vl))
	b.WriteString(`<sl-icon name="chevron-down" class="hf-chev" style="font-size:0.8rem;color:#737373;margin-left:8px"></sl-icon>`)
	b.WriteString(`</div>`)

	b.WriteString(`</div>`) // row
}

// renderHFVariantPanel writes the variant picker panel HTML.
func renderHFVariantPanel(b *strings.Builder, m HFResultModel, modelName string, downloadedFiles map[string]bool) {
	esc := html.EscapeString

	b.WriteString(`<div class="flex items-start justify-between px-3 py-2 bg-neutral-900/60 border-b border-neutral-700">`)
	b.WriteString(`<div class="min-w-0 flex-1">`)
	fmt.Fprintf(b, `<div class="text-sm font-semibold text-neutral-100 truncate">%s</div>`, esc(modelName))
	fmt.Fprintf(b, `<div class="text-xs text-neutral-500 truncate">%s · %s ↓ · %s ♥</div>`,
		esc(m.RepoID), FormatCount(m.Downloads), FormatCount(m.Likes))
	b.WriteString(`</div>`)
	b.WriteString(`<button onclick="window.__hfClose()" title="Close" ` +
		`style="flex-shrink:0;margin-left:8px;padding:2px;line-height:1;background:none;border:none;cursor:pointer;color:#737373">` +
		`<sl-icon name="x-lg" style="font-size:0.85rem;display:block"></sl-icon></button>`)
	b.WriteString(`</div>`)

	if m.Description != "" {
		fmt.Fprintf(b, `<div class="px-3 py-1.5 text-xs text-neutral-400 border-b border-neutral-700/50">%s</div>`,
			esc(Truncate(m.Description, 120)))
	}

	b.WriteString(`<div class="overflow-y-auto" style="max-height:280px">`)
	for _, f := range m.Files {
		quant := ExtractQuant(f.Filename)
		b.WriteString(`<div class="flex items-center gap-2 px-3 py-2 hover:bg-neutral-700/40 border-b border-neutral-700/20 last:border-b-0">`)
		if quant != "" {
			fmt.Fprintf(b, `<span style="min-width:4.5rem;flex-shrink:0;text-align:center" class="font-mono text-xs font-bold bg-neutral-900 text-neutral-200 rounded px-1.5 py-0.5">%s</span>`, esc(quant))
		} else {
			b.WriteString(`<span style="min-width:4.5rem;flex-shrink:0"></span>`)
		}
		fmt.Fprintf(b, `<span class="text-xs text-neutral-300 truncate flex-1" title="%s">%s</span>`, esc(f.Filename), esc(f.Filename))
		fmt.Fprintf(b, `<span class="text-xs text-neutral-500 flex-shrink-0" style="width:4rem;text-align:right">%s</span>`, FormatBytes(f.SizeBytes))
		dlID := FilenameToDlID(f.Filename)
		dlKey := m.RepoID + "/" + f.Filename
		spanAttr := fmt.Sprintf(`id="%s" data-repo="%s" data-file="%s" style="flex-shrink:0;min-width:5.5rem;display:inline-flex;align-items:center;justify-content:center"`,
			esc(dlID), esc(m.RepoID), esc(f.Filename))
		if downloadedFiles[dlKey] {
			fmt.Fprintf(b, `<span %s>`, spanAttr)
			b.WriteString(`<sl-icon name="check-circle-fill" style="color:#22c55e;font-size:1.2rem"></sl-icon>`)
			b.WriteString(`</span>`)
		} else {
			fmt.Fprintf(b, `<span %s>`, spanAttr)
			fmt.Fprintf(b, `<sl-button size="small" variant="primary" onclick="window.__hfDownload('%s','%s')">`,
				esc(m.RepoID), esc(f.Filename))
			b.WriteString(`<sl-icon slot="prefix" name="download"></sl-icon>Get</sl-button>`)
			b.WriteString(`</span>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
}

// RenderHFStatus returns an HTML fragment for the HF search status area.
func RenderHFStatus(msg string, isError bool) string {
	if msg == "" {
		return `<div id="pe-hf-status"></div>`
	}
	variant := "primary"
	if isError {
		variant = "danger"
	}
	return RenderAlert("pe-hf-status", msg, variant, 0)
}
