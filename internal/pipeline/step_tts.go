package pipeline

import (
	"context"
	"fmt"
	"os"

	"moneyprinterFaster/internal/model"
)

// TTSProvider TTS 服务接口
type TTSProvider interface {
	Synthesize(ctx context.Context, text, voiceName string, rate float64, outputPath string) (subMaker any, duration float64, err error)
}

// StepTTS TTS 语音合成步骤
type StepTTS struct {
	provider TTSProvider
}

func NewStepTTS(provider TTSProvider) *StepTTS {
	return &StepTTS{provider: provider}
}

func (s *StepTTS) Name() string { return "tts" }

func (s *StepTTS) Execute(ctx context.Context, task *model.Task, prevData map[string]any) (*StepResult, error) {
	script, ok := prevData["script"].(string)
	if !ok || script == "" {
		return nil, fmt.Errorf("缺少文案数据")
	}

	params := task.Params
	// 构建音频输出路径（任务目录下）
	taskDir := fmt.Sprintf("./data/tasks/%s", task.ID)
	// 确保任务目录存在
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return nil, fmt.Errorf("创建任务目录失败: %w", err)
	}
	audioPath := fmt.Sprintf("%s/audio.mp3", taskDir)

	subMaker, duration, err := s.provider.Synthesize(ctx, script, params.VoiceName, params.VoiceRate, audioPath)
	if err != nil {
		return nil, fmt.Errorf("语音合成失败: %w", err)
	}

	return &StepResult{
		Data: map[string]any{
			"audio_path":     audioPath,
			"audio_duration": duration,
			"sub_maker":      subMaker, // Edge TTS 时间戳数据（用于字幕）
		},
	}, nil
}
