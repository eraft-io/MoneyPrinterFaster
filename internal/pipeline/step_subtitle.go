package pipeline

import (
	"context"
	"fmt"
	"os"

	"moneyprinterFaster/internal/model"
)

// SubtitleProvider 字幕服务接口
type SubtitleProvider interface {
	Generate(ctx context.Context, audioPath string, subMaker any, script string, outputPath string) error
}

// StepSubtitle 字幕生成步骤
type StepSubtitle struct {
	provider SubtitleProvider
}

func NewStepSubtitle(provider SubtitleProvider) *StepSubtitle {
	return &StepSubtitle{provider: provider}
}

func (s *StepSubtitle) Name() string { return "subtitle" }

func (s *StepSubtitle) Execute(ctx context.Context, task *model.Task, prevData map[string]any) (*StepResult, error) {
	params := task.Params

	if !params.SubtitleEnabled {
		return &StepResult{
			Data: map[string]any{
				"subtitle_path": "",
			},
		}, nil
	}

	audioPath, _ := prevData["audio_path"].(string)
	subMaker := prevData["sub_maker"]
	script, _ := prevData["script"].(string)

	taskDir := fmt.Sprintf("./data/tasks/%s", task.ID)
	// 确保任务目录存在
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return nil, fmt.Errorf("创建任务目录失败: %w", err)
	}
	subtitlePath := fmt.Sprintf("%s/subtitle.srt", taskDir)

	if s.provider == nil {
		// 无字幕 provider，跳过
		return &StepResult{
			Data: map[string]any{
				"subtitle_path": "",
			},
		}, nil
	}

	if err := s.provider.Generate(ctx, audioPath, subMaker, script, subtitlePath); err != nil {
		return nil, fmt.Errorf("字幕生成失败: %w", err)
	}

	return &StepResult{
		Data: map[string]any{
			"subtitle_path": subtitlePath,
		},
	}, nil
}
