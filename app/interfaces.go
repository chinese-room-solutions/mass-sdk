// Package app defines the contract every MASS app implements over go-plugin
// gRPC: AppInterface (lifecycle + request dispatch), AppInfo (metadata +
// model requirements), and the typed config structs that mirror MASS's
// config types on the wire.
package app

import (
	"context"
)

// AppInterface is the interface every app must implement.
type AppInterface interface {
	GetInfo() (*AppInfo, error)
	Health() (bool, error)

	// HandleRequest dispatches a unary app-specific RPC. Returns the full
	// response payload as a single []byte.
	HandleRequest(ctx context.Context, method string, payload []byte) ([]byte, error)

	// HandleRequestStream dispatches a streaming app-specific RPC. The app
	// emits zero or more chunks via send and returns when the stream is
	// complete; returning a non-nil error closes the stream with that error.
	//
	// MASS forwards each chunk to the caller as an SSE "data:" line.
	HandleRequestStream(ctx context.Context, method string, payload []byte, send func([]byte) error) error

	// SetLogLevel changes the app's log verbosity at runtime.
	// level is a zerolog level name (e.g. "debug", "info", "warn", "error").
	SetLogLevel(level string) error
}

// AppInfo describes an app and its model requirements.
type AppInfo struct {
	Name        string
	Version     string
	Description string
	Models      []ModelRequirement
}

// ModelRequirement declares a model the app needs MASS to load.
type ModelRequirement struct {
	Name            string
	Type            ModelType
	ChatConfig      *ChatModelConfig
	EmbeddingConfig *EmbeddingModelConfig
}

// ModelType distinguishes chat models from embedding models.
type ModelType int

const (
	ModelTypeChat      ModelType = 0
	ModelTypeEmbedding ModelType = 1
)

// ChatModelConfig mirrors MASS's config.ChatModelConfig.
type ChatModelConfig struct {
	Path          string
	ContextSize   int32
	BatchSize     int32
	Threads       int32
	MaxConcurrent int32
	MaxTokens     int32
	FlashAttn     string
	GpuLayers     int32
	MainGPU       string
	TensorSplit   string
	Thinking      bool
	MmprojPath    string // Vision projector GGUF path (enables vision/multimodal)
}

// EmbeddingModelConfig mirrors MASS's config.EmbeddingModelConfig.
type EmbeddingModelConfig struct {
	Path          string
	ContextSize   int32
	Threads       int32
	MaxConcurrent int32
	GpuLayers     int32
	MainGPU       string
	TensorSplit   string
}
