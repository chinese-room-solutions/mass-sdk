// Package hfui renders the HuggingFace model-search widget: result rows with
// a variant-picker overlay, download buttons wired to the MASS DownloadModel
// endpoint, and the JS runtime that drives download progress. It builds on
// uikit for the generic page kit.
package hfui

import (
	"fmt"
	"html"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/format"
	"github.com/chinese-room-solutions/mass-sdk/ggufutil"
	"github.com/chinese-room-solutions/mass-sdk/uikit"
)

// ResultModel holds data for rendering a HuggingFace search result.
type ResultModel struct {
	RepoID      string
	Description string
	Downloads   int64
	Likes       int64
	Params      int64  // Total parameter count (e.g. 30_000_000_000 for 30B)
	PipelineTag string // HF pipeline_tag, e.g. "text-generation", "feature-extraction"
	AvatarURL   string // Author/org avatar image URL (may be empty)
	Files       []ResultFile
}

// ResultFile holds data for a downloadable GGUF file.
type ResultFile struct {
	Filename  string
	SizeBytes int64
}

// ResultsOpts controls rendering behaviour of HF result helpers.
type ResultsOpts struct {
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
	// SkipFooter disables the built-in Show More footer entirely. Useful
	// when the caller renders its own pagination control bound to a
	// non-Datastar endpoint.
	SkipFooter bool
}

// widgetCSS is the widget's entire presentation, shipped with the rows so the
// embedding page needs no utility-class framework. Every colour comes from the
// --mass-* theme tokens (see uikit/theme.css), so the widget follows whatever
// theme the host page runs. <style> elements, unlike <script>, apply even when
// the rows are injected via innerHTML.
const widgetCSS = `.hf-chev{transition:transform .15s ease}` +
	`[data-open="1"] .hf-chev{transform:rotate(180deg)}` +
	`.hf-list>*+*{margin-top:1px}` +
	`.hf-overlay{background:var(--mass-bg-panel);border:1px solid var(--mass-border);border-radius:.5rem;box-shadow:var(--mass-glow-panel,0 25px 50px -12px rgba(0,0,0,.55))}` +
	`[data-hf-row]{display:flex;align-items:center;gap:.75rem;padding:.625rem .75rem;border-radius:.25rem;cursor:pointer;background:var(--mass-bg-panel);transition:background-color .15s ease}` +
	`[data-hf-row]:hover{background:var(--mass-bg-hover)}` +
	`.hf-avatar{width:2rem;height:2rem;border-radius:9999px;flex-shrink:0;object-fit:cover;background:var(--mass-avatar-bg)}` +
	`div.hf-avatar{display:flex;align-items:center;justify-content:center;color:var(--mass-avatar-text)}` +
	`.hf-grow{flex:1 1 0%;min-width:0}` +
	`.hf-name{font-size:.875rem;font-weight:500;color:var(--mass-text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}` +
	`.hf-meta{font-size:.75rem;color:var(--mass-text-muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}` +
	`.hf-chip{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.75rem;font-weight:700;background:var(--mass-badge-bg);color:var(--mass-badge-text);border-radius:.25rem;padding:.125rem .375rem}` +
	`.hf-name .hf-chip{margin-left:.25rem;vertical-align:1px}` +
	`.hf-end{display:flex;align-items:center;flex-shrink:0}` +
	`.hf-count{font-size:.75rem;background:var(--mass-badge-bg);color:var(--mass-badge-text);border-radius:.25rem;padding:.125rem .375rem}` +
	`.hf-panel-head{display:flex;align-items:flex-start;justify-content:space-between;padding:.5rem .75rem;background:var(--mass-bg-base);border-bottom:1px solid var(--mass-border)}` +
	`.hf-x{flex-shrink:0;margin-left:8px;padding:2px;line-height:1;background:none;border:none;cursor:pointer;color:var(--mass-text-faint)}` +
	`.hf-desc{padding:.375rem .75rem;font-size:.75rem;color:var(--mass-text-muted);border-bottom:1px solid color-mix(in srgb,var(--mass-border) 50%,transparent)}` +
	`.hf-files{overflow-y:auto;max-height:280px}` +
	`.hf-file{display:flex;align-items:center;gap:.5rem;padding:.5rem .75rem;border-bottom:1px solid color-mix(in srgb,var(--mass-border) 30%,transparent)}` +
	`.hf-file:hover{background:var(--mass-bg-hover)}` +
	`.hf-file:last-child{border-bottom:none}` +
	`.hf-file-name{font-size:.75rem;color:var(--mass-text);flex:1 1 0%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}` +
	`.hf-file-size{font-size:.75rem;color:var(--mass-text-muted);flex-shrink:0;width:4rem;text-align:right}` +
	`.hf-footer{margin-top:.5rem;text-align:center}`

// repoToTplID converts a repo ID to a safe HTML template ID.
func repoToTplID(repoID string) string {
	s := strings.ReplaceAll(repoID, "/", "--")
	s = strings.ReplaceAll(s, ".", "_")
	return "hf-tpl-" + s
}

// RenderResults renders the full search results container.
// All models are rendered as visible rows. The server owns pagination state;
// Show More posts to the configured MoreURL to append additional rows via SSE.
func RenderResults(moduleName string, models []ResultModel, opts ResultsOpts) string {
	if len(models) == 0 {
		return `<div id="pe-hf-results"><p style="font-size:.875rem;color:var(--mass-text-muted)">No models found.</p></div>`
	}

	var b strings.Builder
	b.WriteString(`<div id="pe-hf-results">`)

	// Overlay + presentation + JS runtime for the variant picker.
	b.WriteString(`<style>` + widgetCSS + `</style>`)
	b.WriteString(`<div id="hf-panel-overlay" class="hf-overlay" style="display:none;position:fixed;z-index:9999;min-width:340px;max-width:540px;max-height:calc(100vh - 16px);overflow-y:auto;"></div>`)
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
		`el.innerHTML='<div style="position:relative;min-width:4rem;height:1.75rem;border-radius:0.25rem;overflow:hidden;background:var(--mass-bg-active)">` +
		`<div class="hf-dl-bar" style="position:absolute;top:0;left:0;height:100%;width:0%;background:var(--mass-accent-soft);transition:width .3s"></div>` +
		`<span class="hf-dl-pct" style="position:relative;z-index:1;display:flex;align-items:center;justify-content:center;height:100%;font-size:0.75rem;font-weight:600;color:var(--mass-text)">0%</span></div>';` +
		`});` +
		`fetch(dlURL,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({repo_id:repo,filename:file})});` +
		`};` +
		`window.__hfDlProgress=function(file,pct){` +
		`dlEls(window.__hfDlID(file)).forEach(function(el){` +
		`var bar=el.querySelector('.hf-dl-bar');if(bar)bar.style.width=pct+'%';` +
		`var txt=el.querySelector('.hf-dl-pct');if(txt)txt.textContent=pct+'%';` +
		`});};` +
		`window.__hfDlDone=function(file){` +
		`var ck='<sl-icon name="check-circle-fill" style="color:var(--mass-success);font-size:1.2rem"></sl-icon>';` +
		`dlEls(window.__hfDlID(file)).forEach(function(el){el.innerHTML=ck;});` +
		`};` +
		`window.__hfDlErr=function(file,msg){` +
		`var h='<sl-tooltip content="'+msg.replace(/"/g,'&amp;quot;')+'"><sl-icon name="exclamation-triangle-fill" style="color:var(--mass-danger);font-size:1.2rem"></sl-icon></sl-tooltip>';` +
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
	b.WriteString(`<div id="pe-hf-list" class="hf-list">`)
	for _, m := range models {
		renderModelRow(&b, m, opts.DownloadedFiles)
	}
	b.WriteString(`</div>`)

	// Footer with optional Show More — callers that render their own
	// pagination control set SkipFooter=true.
	if !opts.SkipFooter {
		b.WriteString(RenderFooter(moduleName, opts.HasMore, opts.MoreURL))
	}

	b.WriteString(`</div>`) // pe-hf-results
	return b.String()
}

// RenderResultRows renders model rows only (no container), for append-mode
// SSE patches when the user clicks Show More.
func RenderResultRows(models []ResultModel, downloadedFiles map[string]bool) string {
	var b strings.Builder
	for _, m := range models {
		renderModelRow(&b, m, downloadedFiles)
	}
	return b.String()
}

// RenderFooter renders the Show More footer area.
// If hasMore is false the footer is an empty placeholder.
// moreURL overrides the POST URL; when empty, the default module action path is used.
func RenderFooter(moduleName string, hasMore bool, moreURL string) string {
	if !hasMore {
		return `<div id="pe-hf-footer"></div>`
	}
	if moreURL == "" {
		moreURL = "/api/modules/" + html.EscapeString(moduleName) + "/ui/action/show_more_hf"
	}
	return fmt.Sprintf(`<div id="pe-hf-footer" class="hf-footer">`+
		`<sl-button id="pe-hf-more-btn" size="small" variant="text" `+
		`data-on:click="this.disabled=true;this.innerHTML='<sl-spinner style=\'font-size:1rem;--track-width:2px\'></sl-spinner>';@post('%s')">`+
		`<sl-icon slot="prefix" name="chevron-down"></sl-icon>Show More</sl-button></div>`,
		html.EscapeString(moreURL))
}

// renderModelRow writes template + row HTML for a single model.
func renderModelRow(b *strings.Builder, m ResultModel, downloadedFiles map[string]bool) {
	esc := html.EscapeString
	modelName := FormatModelName(m.RepoID)
	tplID := repoToTplID(m.RepoID)

	// Template for variant picker.
	fmt.Fprintf(b, `<template id="%s">`, esc(tplID))
	renderVariantPanel(b, m, modelName, downloadedFiles)
	b.WriteString(`</template>`)

	// Row.
	fmt.Fprintf(b, `<div data-hf-row="1" data-tpl="%s" onclick="window.__hfOpen(this)">`, esc(tplID))

	if m.AvatarURL != "" {
		fmt.Fprintf(b, `<img src="%s" alt="" class="hf-avatar">`, esc(m.AvatarURL))
	} else {
		b.WriteString(`<div class="hf-avatar"><sl-icon name="cpu" style="font-size:1rem"></sl-icon></div>`)
	}

	b.WriteString(`<div class="hf-grow">`)
	fmt.Fprintf(b, `<div class="hf-name">%s`, esc(modelName))
	if m.Params > 0 {
		fmt.Fprintf(b, ` <span class="hf-chip">%s</span>`, esc(format.Params(m.Params)))
	}
	if label, ok := PipelineTagLabel(m.PipelineTag); ok {
		fmt.Fprintf(b, ` <span class="hf-chip">%s</span>`, esc(label))
	}
	if IsVisionPipelineTag(m.PipelineTag) {
		b.WriteString(` <sl-icon name="eye" style="font-size:0.85rem;vertical-align:-2px;color:var(--mass-text-muted);margin-left:6px" title="Vision"></sl-icon>`)
	}
	b.WriteString(`</div>`)
	fmt.Fprintf(b, `<div class="hf-meta">%s · %s ↓ · %s ♥</div>`,
		esc(m.RepoID), format.Count(m.Downloads), format.Count(m.Likes))
	b.WriteString(`</div>`)

	fileCount := len(m.Files)
	vl := fmt.Sprintf("%d variant", fileCount)
	if fileCount != 1 {
		vl += "s"
	}
	b.WriteString(`<div class="hf-end">`)
	fmt.Fprintf(b, `<span class="hf-count">%s</span>`, esc(vl))
	b.WriteString(`<sl-icon name="chevron-down" class="hf-chev" style="font-size:0.8rem;color:var(--mass-text-faint);margin-left:8px"></sl-icon>`)
	b.WriteString(`</div>`)

	b.WriteString(`</div>`) // row
}

// renderVariantPanel writes the variant picker panel HTML.
func renderVariantPanel(b *strings.Builder, m ResultModel, modelName string, downloadedFiles map[string]bool) {
	esc := html.EscapeString

	b.WriteString(`<div class="hf-panel-head">`)
	b.WriteString(`<div class="hf-grow">`)
	fmt.Fprintf(b, `<div class="hf-name">%s</div>`, esc(modelName))
	fmt.Fprintf(b, `<div class="hf-meta">%s · %s ↓ · %s ♥</div>`,
		esc(m.RepoID), format.Count(m.Downloads), format.Count(m.Likes))
	b.WriteString(`</div>`)
	b.WriteString(`<button onclick="window.__hfClose()" title="Close" class="hf-x">` +
		`<sl-icon name="x-lg" style="font-size:0.85rem;display:block"></sl-icon></button>`)
	b.WriteString(`</div>`)

	if m.Description != "" {
		fmt.Fprintf(b, `<div class="hf-desc">%s</div>`, esc(format.Truncate(m.Description, 120)))
	}

	b.WriteString(`<div class="hf-files">`)
	for _, f := range m.Files {
		quant := ggufutil.ExtractQuant(f.Filename)
		b.WriteString(`<div class="hf-file">`)
		if quant != "" {
			fmt.Fprintf(b, `<span class="hf-chip" style="min-width:4.5rem;flex-shrink:0;text-align:center">%s</span>`, esc(quant))
		} else {
			b.WriteString(`<span style="min-width:4.5rem;flex-shrink:0"></span>`)
		}
		fmt.Fprintf(b, `<span class="hf-file-name" title="%s">%s</span>`, esc(f.Filename), esc(f.Filename))
		fmt.Fprintf(b, `<span class="hf-file-size">%s</span>`, format.Bytes(f.SizeBytes))
		dlID := FilenameToDlID(f.Filename)
		dlKey := m.RepoID + "/" + f.Filename
		// flex-end, not center: the slot barely exceeds the Get button's natural
	// width (~61px), and centering would split the slack around it — the row's
	// right gutter must match the left one. The 4rem min-width keeps every
	// state (Get, progress, check, error) the same width so the size column
	// stays aligned across rows; the progress bar in the runtime script must
	// use the same width.
	spanAttr := fmt.Sprintf(`id="%s" data-repo="%s" data-file="%s" style="flex-shrink:0;min-width:4rem;display:inline-flex;align-items:center;justify-content:flex-end"`,
			esc(dlID), esc(m.RepoID), esc(f.Filename))
		if downloadedFiles[dlKey] {
			fmt.Fprintf(b, `<span %s>`, spanAttr)
			b.WriteString(`<sl-icon name="check-circle-fill" style="color:var(--mass-success);font-size:1.2rem"></sl-icon>`)
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

// RenderStatus returns an HTML fragment for the HF search status area.
func RenderStatus(msg string, isError bool) string {
	if msg == "" {
		return `<div id="pe-hf-status"></div>`
	}
	variant := "primary"
	if isError {
		variant = "danger"
	}
	return uikit.RenderAlert("pe-hf-status", msg, variant, 0)
}
