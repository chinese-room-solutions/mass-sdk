package hfui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
