package uikit

import (
	"fmt"
	"strings"
)

// FormatParams formats a parameter count as a human-readable string (e.g. "7B", "1.5B", "400M").
func FormatParams(n int64) string {
	switch {
	case n >= 1_000_000_000:
		v := float64(n) / 1_000_000_000
		if v == float64(int64(v)) {
			return fmt.Sprintf("%dB", int64(v))
		}
		return fmt.Sprintf("%.1fB", v)
	case n >= 1_000_000:
		v := float64(n) / 1_000_000
		if v == float64(int64(v)) {
			return fmt.Sprintf("%dM", int64(v))
		}
		return fmt.Sprintf("%.0fM", v)
	default:
		return fmt.Sprintf("%dK", n/1_000)
	}
}

// IsQuantToken checks whether a token (already upper-cased) looks like a GGUF quantization tag.
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

// ExtractQuant extracts the quantization tag from a GGUF filename.
func ExtractQuant(filename string) string {
	base := strings.TrimSuffix(strings.ToLower(filename), ".gguf")
	parts := strings.FieldsFunc(base, func(r rune) bool { return r == '.' || r == '-' })
	for i := len(parts) - 1; i >= 0; i-- {
		p := strings.ToUpper(parts[i])
		if len(p) >= 2 && IsQuantToken(p) {
			return p
		}
	}
	return ""
}

// knownSuffixes are trailing tokens to strip from model repo names.
var knownSuffixes = []string{"-GGUF", "_GGUF", "-gguf", "_gguf"}

// FormatModelName converts a HuggingFace repo ID into a human-readable model name.
func FormatModelName(repoID string) string {
	name := repoID
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}

	upper := strings.ToUpper(name)
	for _, suf := range knownSuffixes {
		if strings.HasSuffix(upper, strings.ToUpper(suf)) {
			name = name[:len(name)-len(suf)]
			break
		}
	}

	tokens := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, t := range tokens {
		if t == "" {
			continue
		}
		allLower := true
		for _, r := range t {
			if r < 'a' || r > 'z' {
				allLower = false
				break
			}
		}
		if allLower {
			tokens[i] = strings.ToUpper(t[:1]) + t[1:]
		}
	}
	return strings.Join(tokens, " ")
}

// PipelineTagMap maps HuggingFace pipeline_tag values to short display labels.
var PipelineTagMap = map[string]string{
	"text-generation":              "Chat",
	"text2text-generation":         "T2T",
	"feature-extraction":           "Embed",
	"sentence-similarity":          "Embed",
	"fill-mask":                    "MLM",
	"question-answering":           "QA",
	"text-classification":          "Class",
	"token-classification":         "NER",
	"translation":                  "Trans",
	"summarization":                "Sum",
	"image-text-to-text":           "Chat",
	"visual-question-answering":    "Chat",
	"image-to-text":                "Chat",
	"image-classification":         "Image",
	"object-detection":             "Detect",
	"text-to-image":                "ImgGen",
	"image-to-image":               "ImgGen",
	"text-to-audio":                "Audio",
	"text-to-speech":               "TTS",
	"automatic-speech-recognition": "ASR",
	"audio-classification":         "Audio",
}

// VisionPipelineTags is the set of pipeline tags that indicate vision/multimodal capability.
var VisionPipelineTags = map[string]bool{
	"image-text-to-text":        true,
	"visual-question-answering": true,
	"image-to-text":             true,
}
