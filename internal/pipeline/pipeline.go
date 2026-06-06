package pipeline

import (
	"context"
	"fmt"
	"log"

	"moneyprinterFaster/internal/model"
)

// Pipeline 视频生成流水线
type Pipeline struct {
	steps []Step
}

// NewPipeline 创建 Pipeline
func NewPipeline(steps ...Step) *Pipeline {
	return &Pipeline{steps: steps}
}

// Run 执行 Pipeline（满足 worker.PipelineRunner 接口）
func (p *Pipeline) Run(ctx context.Context, task *model.Task, onProgress func(float64, string)) error {
	data := make(map[string]any)
	totalSteps := len(p.steps)
	if totalSteps == 0 {
		return fmt.Errorf("pipeline 无步骤")
	}

	stepWeight := 100.0 / float64(totalSteps)

	for i, step := range p.steps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("[Pipeline] 任务 %s: 执行步骤 %d/%d [%s]", task.ID, i+1, totalSteps, step.Name())

		result, err := step.Execute(ctx, task, data)
		if err != nil {
			return fmt.Errorf("步骤 %s 失败: %w", step.Name(), err)
		}

		// 合并产出到 data
		for k, v := range result.Data {
			data[k] = v
		}

		progress := float64(i+1) * stepWeight
		onProgress(progress, step.Name())
		log.Printf("[Pipeline] 任务 %s: 步骤 [%s] 完成 (%.1f%%)", task.ID, step.Name(), progress)
	}

	// 将最终输出路径和时长写回 task
	if outputPath, ok := data["final_video_path"].(string); ok {
		task.OutputPath = outputPath
	}
	if duration, ok := data["video_duration"].(float64); ok {
		task.VideoDuration = duration
	}

	return nil
}
