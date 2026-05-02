package uikit

import (
	"strings"
)

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
