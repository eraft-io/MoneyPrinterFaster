package model

import (
	"context"
	"time"
)

// TaskPriority 任务优先级
type TaskPriority int

const (
	PriorityHigh   TaskPriority = 0
	PriorityNormal TaskPriority = 1
	PriorityLow    TaskPriority = 2
)

// TaskState 任务状态
type TaskState string

const (
	StatePending    TaskState = "pending"
	StateProcessing TaskState = "processing"
	StateCompleted  TaskState = "completed"
	StateFailed     TaskState = "failed"
	StateCancelled  TaskState = "cancelled"
)

// IsTerminal 判断是否为终态
func (s TaskState) IsTerminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCancelled
}

// Task 任务结构
type Task struct {
	ID            string       `json:"id"`
	State         TaskState    `json:"state"`
	Priority      TaskPriority `json:"priority"`
	Params        VideoParams  `json:"params"`
	Progress      float64      `json:"progress"`
	Error         string       `json:"error,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	StartedAt     *time.Time   `json:"started_at,omitempty"`
	FinishedAt    *time.Time   `json:"finished_at,omitempty"`
	OutputPath    string       `json:"output_path,omitempty"`
	VideoDuration float64      `json:"video_duration,omitempty"` // 最终视频时长（秒）

	// 运行时字段，不参与序列化
	CancelFunc context.CancelFunc `json:"-"`
}

// MarkProcessing 标记为处理中
func (t *Task) MarkProcessing() {
	now := time.Now()
	t.State = StateProcessing
	t.StartedAt = &now
	t.Progress = 0
}

// MarkCompleted 标记为完成
func (t *Task) MarkCompleted(outputPath string) {
	now := time.Now()
	t.State = StateCompleted
	t.FinishedAt = &now
	t.Progress = 100
	t.OutputPath = outputPath
}

// MarkFailed 标记为失败
func (t *Task) MarkFailed(err error) {
	now := time.Now()
	t.State = StateFailed
	t.FinishedAt = &now
	if err != nil {
		t.Error = err.Error()
	}
}

// MarkCancelled 标记为已取消
func (t *Task) MarkCancelled() {
	now := time.Now()
	t.State = StateCancelled
	t.FinishedAt = &now
}

// Duration 返回任务耗时（若未开始则返回 0）
func (t *Task) Duration() time.Duration {
	if t.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if t.FinishedAt != nil {
		end = *t.FinishedAt
	}
	return end.Sub(*t.StartedAt)
}
