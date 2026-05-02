package ggufutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractQuant(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"dot-separated quant", "llava-v1.6-mistral-7b.Q4_K_M.gguf", "Q4_K_M"},
		{"dash-separated quant", "Qwen2.5-7B-Instruct-Q4_K_M.gguf", "Q4_K_M"},
		{"large K_XL variant with UD prefix", "Qwen3.5-4B-UD-Q4_K_XL.gguf", "UD-Q4_K_XL"},
		{"F16", "model-F16.gguf", "F16"},
		{"BF16", "model-BF16.gguf", "BF16"},
		{"IQ2_XXS", "model-IQ2_XXS.gguf", "IQ2_XXS"},
		{"MXFP4", "model-MXFP4_MOE.gguf", "MXFP4_MOE"},
		{"no quant", "random-model.gguf", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ExtractQuant(tt.filename))
		})
	}
}

func TestIsQuantToken(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"Q4", true},
		{"Q8_0", true},
		{"Q4_K_M", true},
		{"F16", true},
		{"BF16", true},
		{"IQ2_XXS", true},
		{"MXFP4", true},
		{"Q", false},
		{"K", false},
		{"INSTRUCT", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			require.Equal(t, tt.want, IsQuantToken(tt.token))
		})
	}
}
