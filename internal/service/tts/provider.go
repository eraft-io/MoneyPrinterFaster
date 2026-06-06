package tts

import (
	"context"
	"fmt"

	"moneyprinterFaster/internal/config"
)

// Provider TTS 服务接口
type Provider interface {
	Synthesize(ctx context.Context, text, voiceName string, rate float64, outputPath string) (subMaker any, duration float64, err error)
}

// NewProvider 根据配置创建 TTS Provider
func NewProvider(cfg *config.TTSConfig) (Provider, error) {
	switch cfg.Provider {
	case "edge":
		return NewEdgeTTS(), nil
	case "azure":
		return NewAzureTTS(cfg.Azure.SpeechKey, cfg.Azure.SpeechRegion), nil
	case "openai":
		return NewOpenAITTS(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model, cfg.OpenAI.Voice), nil
	default:
		return nil, fmt.Errorf("不支持的 TTS provider: %s", cfg.Provider)
	}
}
