package tts

import (
	"context"
	"fmt"
	"io"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAITTS OpenAI TTS Provider
type OpenAITTS struct {
	client *openai.Client
	model  string
	voice  string
}

func NewOpenAITTS(apiKey, baseURL, model, voice string) *OpenAITTS {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	if model == "" {
		model = "tts-1"
	}
	if voice == "" {
		voice = "alloy"
	}
	return &OpenAITTS{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
		voice:  voice,
	}
}

func (o *OpenAITTS) Synthesize(ctx context.Context, text, voiceName string, rate float64, outputPath string) (any, float64, error) {
	voice := o.voice
	if voiceName != "" {
		voice = voiceName
	}

	resp, err := o.client.CreateSpeech(ctx, openai.CreateSpeechRequest{
		Model: openai.SpeechModel(o.model),
		Input: text,
		Voice: openai.SpeechVoice(voice),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("OpenAI TTS API 调用失败: %w", err)
	}
	defer resp.Close()

	outFile, err := os.Create(outputPath)
	if err != nil {
		return nil, 0, fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp); err != nil {
		return nil, 0, fmt.Errorf("写入音频文件失败: %w", err)
	}

	return nil, 0, nil
}
