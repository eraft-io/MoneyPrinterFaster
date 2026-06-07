package pipeline

import (
	"context"
	"fmt"
	"log"

	"moneyprinterFaster/internal/model"
)

// ComposeResult 合成结果
type ComposeResult struct {
	OutputPath string
	Duration   float64 // 最终视频时长（秒）
}

// ComposeService 视频合成服务接口
type ComposeService interface {
	Compose(ctx context.Context, opts ComposeOpts) (ComposeResult, error)
}

// ComposeOpts 合成参数
type ComposeOpts struct {
	TaskDir      string
	VideoPaths   []string
	AudioPath    string
	SubtitlePath string
	BGMPath      string
	Params       model.VideoParams
	OutputIndex  int
	IsImageMode  bool // 素材为图片模式（自动转换为视频片段）
	ClipSeconds  int  // 图片模式下每张图的展示时长（秒）
}

// StepCompose 视频合成步骤
type StepCompose struct {
	service ComposeService
}

func NewStepCompose(service ComposeService) *StepCompose {
	return &StepCompose{service: service}
}

func (s *StepCompose) Name() string { return "compose" }

func (s *StepCompose) Execute(ctx context.Context, task *model.Task, prevData map[string]any) (*StepResult, error) {
	videoPaths, _ := prevData["video_paths"].([]string)
	audioPath, _ := prevData["audio_path"].(string)
	subtitlePath, _ := prevData["subtitle_path"].(string)
	isImageMode, _ := prevData["is_image_mode"].(bool)
	clipSeconds, _ := prevData["clip_seconds"].(int)
	if clipSeconds <= 0 {
		clipSeconds = 5
	}

	log.Printf("[StepCompose] 开始视频合成: task=%s, image_mode=%v, clip_seconds=%d", task.ID, isImageMode, clipSeconds)
	log.Printf("[StepCompose]   视频素材: %d 个", len(videoPaths))
	for i, p := range videoPaths {
		log.Printf("[StepCompose]     [%d] %s", i+1, p)
	}
	log.Printf("[StepCompose]   音频文件: %s", audioPath)
	log.Printf("[StepCompose]   字幕文件: %s", subtitlePath)
	log.Printf("[StepCompose]   BGM: enabled=%v, file=%s", task.Params.BGMEnabled, task.Params.BGMFile)
	log.Printf("[StepCompose]   视频数量: %d, 分辨率: %dx%d",
		task.Params.VideoCount, task.Params.Width(), task.Params.Height())

	if len(videoPaths) == 0 {
		return nil, fmt.Errorf("缺少视频素材")
	}
	if audioPath == "" {
		return nil, fmt.Errorf("缺少音频文件")
	}

	params := task.Params
	taskDir := fmt.Sprintf("./data/tasks/%s", task.ID)

	var finalPaths []string
	var mainOutput string
	var mainDuration float64

	for i := 0; i < params.VideoCount; i++ {
		log.Printf("[StepCompose] 开始合成第 %d/%d 个视频...", i+1, params.VideoCount)
		opts := ComposeOpts{
			TaskDir:      taskDir,
			VideoPaths:   videoPaths,
			AudioPath:    audioPath,
			SubtitlePath: subtitlePath,
			BGMPath:      params.BGMFile,
			Params:       params,
			OutputIndex:  i + 1,
			IsImageMode:  isImageMode,
			ClipSeconds:  clipSeconds,
		}

		result, err := s.service.Compose(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("视频合成 #%d 失败: %w", i+1, err)
		}
		log.Printf("[StepCompose] 第 %d/%d 个视频合成完成: %s (duration=%.1fs)", i+1, params.VideoCount, result.OutputPath, result.Duration)
		finalPaths = append(finalPaths, result.OutputPath)
		if i == 0 {
			mainOutput = result.OutputPath
			mainDuration = result.Duration
		}
	}

	// 确保主输出有值
	if mainOutput == "" && len(finalPaths) > 0 {
		mainOutput = finalPaths[0]
	}

	log.Printf("[StepCompose] 视频合成全部完成: task=%s, output=%s, duration=%.1fs", task.ID, mainOutput, mainDuration)

	return &StepResult{
		Data: map[string]any{
			"final_video_path": mainOutput,
			"all_video_paths":  finalPaths,
			"video_duration":   mainDuration,
		},
	}, nil
}
