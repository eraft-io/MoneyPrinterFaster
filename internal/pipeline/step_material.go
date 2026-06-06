package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"moneyprinterFaster/internal/model"
)

// MaterialProvider 素材服务接口
type MaterialProvider interface {
	Download(ctx context.Context, terms []string, aspect model.VideoAspect, duration float64, outputDir string) ([]string, error)
}

// StepMaterial 素材采集步骤
type StepMaterial struct {
	provider MaterialProvider
}

func NewStepMaterial(provider MaterialProvider) *StepMaterial {
	return &StepMaterial{provider: provider}
}

func (s *StepMaterial) Name() string { return "material" }

func (s *StepMaterial) Execute(ctx context.Context, task *model.Task, prevData map[string]any) (*StepResult, error) {
	terms, _ := prevData["terms"].([]string)
	audioDuration, _ := prevData["audio_duration"].(float64)

	log.Printf("[StepMaterial] 开始素材采集: task=%s, keywords=%v, audio_duration=%.1fs",
		task.ID, terms, audioDuration)

	if len(terms) == 0 {
		return nil, fmt.Errorf("缺少搜索关键词")
	}

	if s.provider == nil {
		return nil, fmt.Errorf("素材服务未配置，无法下载视频素材。请在 config.toml 中配置 [material.pexels] api_keys")
	}

	params := task.Params
	taskDir := fmt.Sprintf("./data/tasks/%s", task.ID)
	materialDir := fmt.Sprintf("%s/materials", taskDir)

	// 确保素材目录存在
	if err := os.MkdirAll(materialDir, 0755); err != nil {
		return nil, fmt.Errorf("创建素材目录失败: %w", err)
	}

	// 素材只需要覆盖一次音频时长即可（compose 会复用同一批素材）
	// 加 10% 余量确保素材够用
	totalDuration := audioDuration * 1.1
	log.Printf("[StepMaterial] 需要素材时长: %.1fs (audio=%.1fs × 1.1 余量), video_count=%d, aspect=%s",
		totalDuration, audioDuration, params.VideoCount, params.Aspect)

	videoPaths, err := s.provider.Download(ctx, terms, params.Aspect, totalDuration, materialDir)
	if err != nil {
		return nil, fmt.Errorf("素材下载失败: %w", err)
	}

	if len(videoPaths) == 0 {
		return nil, fmt.Errorf("未获取到任何素材")
	}

	log.Printf("[StepMaterial] 素材采集完成: 共 %d 个视频文件", len(videoPaths))
	for i, p := range videoPaths {
		log.Printf("[StepMaterial]   [%d] %s", i+1, p)
	}

	return &StepResult{
		Data: map[string]any{
			"video_paths":  videoPaths,
			"material_dir": materialDir,
		},
	}, nil
}

// formatTerms 格式化关键词用于日志
func formatTerms(terms []string) string {
	return strings.Join(terms, ", ")
}
