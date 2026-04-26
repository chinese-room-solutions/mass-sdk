package app

import pb "github.com/chinese-room-solutions/mass-sdk/app/rpc"

func appInfoToProto(info *AppInfo) *pb.AppInfo {
	models := make([]*pb.ModelRequirement, len(info.Models))
	for i, m := range info.Models {
		req := &pb.ModelRequirement{
			Name: m.Name,
			Type: pb.ModelType(m.Type),
		}
		if m.ChatConfig != nil {
			req.ChatConfig = &pb.ChatModelConfig{
				Path:          m.ChatConfig.Path,
				ContextSize:   m.ChatConfig.ContextSize,
				BatchSize:     m.ChatConfig.BatchSize,
				Threads:       m.ChatConfig.Threads,
				MaxConcurrent: m.ChatConfig.MaxConcurrent,
				MaxTokens:     m.ChatConfig.MaxTokens,
				FlashAttn:     m.ChatConfig.FlashAttn,
				GpuLayers:     m.ChatConfig.GpuLayers,
				MainGpu:       m.ChatConfig.MainGPU,
				TensorSplit:   m.ChatConfig.TensorSplit,
				Thinking:      m.ChatConfig.Thinking,
				MmprojPath:    m.ChatConfig.MmprojPath,
			}
		}
		if m.EmbeddingConfig != nil {
			req.EmbeddingConfig = &pb.EmbeddingModelConfig{
				Path:          m.EmbeddingConfig.Path,
				ContextSize:   m.EmbeddingConfig.ContextSize,
				Threads:       m.EmbeddingConfig.Threads,
				MaxConcurrent: m.EmbeddingConfig.MaxConcurrent,
				GpuLayers:     m.EmbeddingConfig.GpuLayers,
				MainGpu:       m.EmbeddingConfig.MainGPU,
				TensorSplit:   m.EmbeddingConfig.TensorSplit,
			}
		}
		models[i] = req
	}
	return &pb.AppInfo{
		Name:        info.Name,
		Version:     info.Version,
		Description: info.Description,
		Models:      models,
	}
}

func protoToAppInfo(p *pb.AppInfo) *AppInfo {
	models := make([]ModelRequirement, len(p.Models))
	for i, m := range p.Models {
		req := ModelRequirement{
			Name: m.Name,
			Type: ModelType(m.Type),
		}
		if m.ChatConfig != nil {
			req.ChatConfig = &ChatModelConfig{
				Path:          m.ChatConfig.Path,
				ContextSize:   m.ChatConfig.ContextSize,
				BatchSize:     m.ChatConfig.BatchSize,
				Threads:       m.ChatConfig.Threads,
				MaxConcurrent: m.ChatConfig.MaxConcurrent,
				MaxTokens:     m.ChatConfig.MaxTokens,
				FlashAttn:     m.ChatConfig.FlashAttn,
				GpuLayers:     m.ChatConfig.GpuLayers,
				MainGPU:       m.ChatConfig.MainGpu,
				TensorSplit:   m.ChatConfig.TensorSplit,
				Thinking:      m.ChatConfig.Thinking,
				MmprojPath:    m.ChatConfig.MmprojPath,
			}
		}
		if m.EmbeddingConfig != nil {
			req.EmbeddingConfig = &EmbeddingModelConfig{
				Path:          m.EmbeddingConfig.Path,
				ContextSize:   m.EmbeddingConfig.ContextSize,
				Threads:       m.EmbeddingConfig.Threads,
				MaxConcurrent: m.EmbeddingConfig.MaxConcurrent,
				GpuLayers:     m.EmbeddingConfig.GpuLayers,
				MainGPU:       m.EmbeddingConfig.MainGpu,
				TensorSplit:   m.EmbeddingConfig.TensorSplit,
			}
		}
		models[i] = req
	}
	return &AppInfo{
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		Models:      models,
	}
}
