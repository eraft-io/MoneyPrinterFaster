package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"moneyprinterFaster/internal/model"
	"moneyprinterFaster/internal/service/material"
)

// MaterialProvider 素材服务接口
type MaterialProvider interface {
	Download(ctx context.Context, terms []string, aspect model.VideoAspect, duration float64, outputDir string) ([]string, error)
}

// StepMaterial 素材采集步骤
type StepMaterial struct {
	providers     map[string]MaterialProvider // source name -> provider
	defaultSource string                      // 默认素材来源
}

// NewStepMaterial 创建素材步骤（兼容旧单 provider 模式）
func NewStepMaterial(provider MaterialProvider) *StepMaterial {
	return &StepMaterial{
		providers:     map[string]MaterialProvider{"default": provider},
		defaultSource: "default",
	}
}

// NewStepMaterialMulti 创建素材步骤（多 provider 模式）
func NewStepMaterialMulti(providers map[string]MaterialProvider, defaultSource string) *StepMaterial {
	if defaultSource == "" {
		defaultSource = "default"
	}
	return &StepMaterial{
		providers:     providers,
		defaultSource: defaultSource,
	}
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

	// 选择素材 provider：优先使用任务指定的 source，否则使用默认
	sourceKey := task.Params.MaterialSource
	if sourceKey == "" {
		sourceKey = s.defaultSource
	}

	provider, ok := s.providers[sourceKey]
	if !ok {
		// 尝试 fallback 到默认
		provider, ok = s.providers[s.defaultSource]
		if !ok {
			return nil, fmt.Errorf("素材服务未配置: %s。可用: %v", sourceKey, s.availableSources())
		}
		log.Printf("[StepMaterial] 素材来源 %q 不可用，回退到默认 %q", sourceKey, s.defaultSource)
		sourceKey = s.defaultSource
	}
	log.Printf("[StepMaterial] 使用素材来源: %s", sourceKey)

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

	// 判断是否为图片模式（通过检查 provider 类型）
	isImageMode := false
	clipSeconds := 5 // 默认每张图片展示时长
	if p, ok := provider.(*material.AliyunImage); ok {
		isImageMode = true
		clipSeconds = p.ClipSeconds() // 从 provider 获取配置
		log.Printf("[StepMaterial] 检测到图片模式，素材将由 composer 转换为视频片段 (clip_seconds=%d)", clipSeconds)
	}

	videoPaths, err := provider.Download(ctx, terms, params.Aspect, totalDuration, materialDir)
	if err != nil {
		return nil, fmt.Errorf("素材下载失败: %w", err)
	}

	if len(videoPaths) == 0 {
		return nil, fmt.Errorf("未获取到任何素材")
	}

	log.Printf("[StepMaterial] 素材采集完成: 共 %d 个文件 (图片模式=%v)", len(videoPaths), isImageMode)
	for i, p := range videoPaths {
		log.Printf("[StepMaterial]   [%d] %s", i+1, p)
	}

	return &StepResult{
		Data: map[string]any{
			"video_paths":   videoPaths,
			"material_dir":  materialDir,
			"is_image_mode": isImageMode,
			"clip_seconds":  clipSeconds,
		},
	}, nil
}

// availableSources 返回所有可用的素材来源名称
func (s *StepMaterial) availableSources() []string {
	sources := make([]string, 0, len(s.providers))
	for k := range s.providers {
		sources = append(sources, k)
	}
	return sources
}

// formatTerms 格式化关键词用于日志
func formatTerms(terms []string) string {
	return strings.Join(terms, ", ")
}
