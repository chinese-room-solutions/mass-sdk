// Package ggufutil provides pure-Go helpers for working with GGUF model
// filenames. Pure Go, no UI/HTML dependencies — safe to import from any
// backend, gateway, worker, or CLI.
package ggufutil

import "strings"

// ExtractQuant pulls the trailing quant tag from a GGUF filename
// (e.g. "model-Q4_K_M.gguf" → "Q4_K_M"). Preserves community-used
// prefixes that travel with the quant ("Qwen3.5-4B-UD-Q4_K_XL.gguf"
// → "UD-Q4_K_XL"). Returns "" when no quant pattern is found.
//
// Splits on '.' and '-' (not '_', so multi-word tags like "Q4_K_M" stay
// intact), walks parts right-to-left, finds the rightmost quant-shaped
// token per [IsQuantToken], then absorbs any short ALL-CAPS modifier
// tokens immediately to the left (recognised by [IsQuantPrefixToken])
// so labels like "UD-Q4_K_XL" stay whole.
func ExtractQuant(filename string) string {
	base := strings.TrimSuffix(filename, ".gguf")
	parts := strings.FieldsFunc(base, func(r rune) bool { return r == '.' || r == '-' })
	quantIdx := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if IsQuantToken(strings.ToUpper(parts[i])) {
			quantIdx = i
			break
		}
	}
	if quantIdx < 0 {
		return ""
	}
	for quantIdx > 0 && IsQuantPrefixToken(strings.ToUpper(parts[quantIdx-1])) {
		quantIdx--
	}
	out := make([]string, 0, len(parts)-quantIdx)
	for _, p := range parts[quantIdx:] {
		out = append(out, strings.ToUpper(p))
	}
	return strings.Join(out, "-")
}

// IsQuantPrefixToken reports whether an upper-cased token is a known
// community modifier that prepends a quant tag (e.g. unsloth's "UD-").
// Conservative list — only documented prefixes belong here.
func IsQuantPrefixToken(p string) bool {
	switch p {
	case "UD":
		return true
	}
	return false
}

// IsQuantToken reports whether an upper-cased token looks like a known GGUF
// quantization tag: starts with Q + digit, F + digit, BF + digit, IQ + digit,
// or "MXFP" prefix.
func IsQuantToken(p string) bool {
	if len(p) < 2 {
		return false
	}
	if p[0] == 'Q' && p[1] >= '0' && p[1] <= '9' {
		return true
	}
	if p[0] == 'F' && p[1] >= '0' && p[1] <= '9' {
		return true
	}
	if len(p) >= 3 && p[0] == 'B' && p[1] == 'F' && p[2] >= '0' && p[2] <= '9' {
		return true
	}
	if len(p) >= 3 && p[0] == 'I' && p[1] == 'Q' && p[2] >= '0' && p[2] <= '9' {
		return true
	}
	return strings.HasPrefix(p, "MXFP")
}
