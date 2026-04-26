package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppInfoRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		info *AppInfo
	}{
		{
			name: "minimal info",
			info: &AppInfo{
				Name:    "test-app",
				Version: "1.0.0",
				Models:  []ModelRequirement{},
			},
		},
		{
			name: "with chat model",
			info: &AppInfo{
				Name:        "chat-app",
				Version:     "2.0.0",
				Description: "A chat app",
				Models: []ModelRequirement{
					{
						Name: "main",
						Type: ModelTypeChat,
						ChatConfig: &ChatModelConfig{
							Path:          "/models/chat.gguf",
							ContextSize:   4096,
							BatchSize:     512,
							Threads:       8,
							MaxConcurrent: 4,
							MaxTokens:     2048,
							FlashAttn:     "auto",
							GpuLayers:     99,
							MainGPU:       "0",
							TensorSplit:   "50,50",
							Thinking:      true,
						},
					},
				},
			},
		},
		{
			name: "with embedding model",
			info: &AppInfo{
				Name:    "embed-app",
				Version: "1.0.0",
				Models: []ModelRequirement{
					{
						Name: "embedding",
						Type: ModelTypeEmbedding,
						EmbeddingConfig: &EmbeddingModelConfig{
							Path:          "/models/embed.gguf",
							ContextSize:   8192,
							Threads:       4,
							MaxConcurrent: 8,
							GpuLayers:     33,
							MainGPU:       "1",
							TensorSplit:   "70,30",
						},
					},
				},
			},
		},
		{
			name: "mixed models",
			info: &AppInfo{
				Name:    "multi-app",
				Version: "3.0.0",
				Models: []ModelRequirement{
					{
						Name: "chat",
						Type: ModelTypeChat,
						ChatConfig: &ChatModelConfig{
							Path:        "/models/chat.gguf",
							ContextSize: 2048,
							Threads:     4,
						},
					},
					{
						Name: "embed",
						Type: ModelTypeEmbedding,
						EmbeddingConfig: &EmbeddingModelConfig{
							Path:        "/models/embed.gguf",
							ContextSize: 512,
							Threads:     2,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto := appInfoToProto(tt.info)
			require.NotNil(t, proto)

			result := protoToAppInfo(proto)
			require.Equal(t, tt.info, result)
		})
	}
}

func TestAppInfoToProtoNilConfigs(t *testing.T) {
	info := &AppInfo{
		Name:    "test",
		Version: "1.0.0",
		Models: []ModelRequirement{
			{Name: "m1", Type: ModelTypeChat},
		},
	}
	proto := appInfoToProto(info)
	require.Len(t, proto.Models, 1)
	require.Nil(t, proto.Models[0].ChatConfig)
	require.Nil(t, proto.Models[0].EmbeddingConfig)

	result := protoToAppInfo(proto)
	require.Nil(t, result.Models[0].ChatConfig)
	require.Nil(t, result.Models[0].EmbeddingConfig)
}
