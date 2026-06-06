package tts

import (
	"context"
	"fmt"
)

// AzureTTS Azure Cognitive Services TTS Provider
type AzureTTS struct {
	speechKey    string
	speechRegion string
}

func NewAzureTTS(speechKey, speechRegion string) *AzureTTS {
	return &AzureTTS{
		speechKey:    speechKey,
		speechRegion: speechRegion,
	}
}

func (a *AzureTTS) Synthesize(ctx context.Context, text, voiceName string, rate float64, outputPath string) (any, float64, error) {
	if a.speechKey == "" {
		return nil, 0, fmt.Errorf("Azure Speech Key 未配置")
	}
	if a.speechRegion == "" {
		a.speechRegion = "eastus"
	}

	// TODO: 集成 Azure Cognitive Services Speech SDK
	// 当前为骨架实现，后续接入 azure-cognitiveservices-speech Go SDK
	return nil, 0, fmt.Errorf("Azure TTS 尚未实现，请使用 edge provider")
}
