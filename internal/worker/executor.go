package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"moneyprinterFaster/internal/model"
	"moneyprinterFaster/internal/queue"
)

// 默认任务超时：单个任务最多执行 15 分钟
const defaultTaskTimeout = 15 * time.Minute

// Executor 任务执行器
type Executor struct {
	queue    queue.Queue
	pipeline PipelineRunner
}

// PipelineRunner Pipeline 运行接口（解耦 worker 和 pipeline 包）
type PipelineRunner interface {
	Run(ctx context.Context, task *model.Task, onProgress func(float64, string)) error
}

// NewExecutor 创建执行器
func NewExecutor(q queue.Queue) *Executor {
	return &Executor{
		queue: q,
	}
}

// SetPipeline 注入 Pipeline（在 main 中初始化后调用）
func (e *Executor) SetPipeline(p PipelineRunner) {
	e.pipeline = p
}

// Execute 执行单个任务（带超时 + panic 恢复）
func (e *Executor) Execute(parentCtx context.Context, task *model.Task) {
	// 为任务创建带超时的 context
	ctx, cancel := context.WithTimeout(parentCtx, defaultTaskTimeout)
	defer cancel()

	// 设置任务状态为处理中
	e.queue.SetState(task.ID, model.StateProcessing, "")
	// 保存 cancel 函数以便外部取消
	task.CancelFunc = cancel

	log.Printf("[Executor] 开始执行任务 %s (超时: %s)", task.ID, defaultTaskTimeout)

	// 确保无论发生什么，任务都会达到终态
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("任务 panic: %v", r)
			log.Printf("[Executor] 任务 %s panic: %v", task.ID, r)
			task.MarkFailed(fmt.Errorf("%s", errMsg))
			e.queue.SetState(task.ID, model.StateFailed, errMsg)
		}
	}()

	if e.pipeline == nil {
		errMsg := "pipeline 未初始化"
		task.MarkFailed(fmt.Errorf("%s", errMsg))
		e.queue.SetState(task.ID, model.StateFailed, errMsg)
		log.Printf("[Executor] 任务 %s 失败: %s", task.ID, errMsg)
		return
	}

	// 运行 Pipeline
	err := e.pipeline.Run(ctx, task, func(progress float64, step string) {
		task.Progress = progress
		e.queue.UpdateProgress(task.ID, progress, step)
	})

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			errMsg := fmt.Sprintf("任务超时 (%s)", defaultTaskTimeout)
			task.MarkFailed(fmt.Errorf("%s", errMsg))
			e.queue.SetState(task.ID, model.StateFailed, errMsg)
			log.Printf("[Executor] 任务 %s 超时", task.ID)
		} else if ctx.Err() != nil {
			task.MarkCancelled()
			e.queue.SetState(task.ID, model.StateCancelled, "任务已取消")
			log.Printf("[Executor] 任务 %s 已取消", task.ID)
		} else {
			errMsg := err.Error()
			task.MarkFailed(err)
			e.queue.SetState(task.ID, model.StateFailed, errMsg)
			log.Printf("[Executor] 任务 %s 失败: %s", task.ID, errMsg)
		}
		return
	}

	// 成功
	task.MarkCompleted(task.OutputPath)
	e.queue.SetState(task.ID, model.StateCompleted, "")
	log.Printf("[Executor] 任务 %s 完成，输出: %s", task.ID, task.OutputPath)
}
