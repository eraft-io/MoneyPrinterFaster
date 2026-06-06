package model

import (
	"context"
	"encoding/json"
	"math"
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

// CostSeconds 返回任务消耗的秒数（0 表示未完成或未开始）
func (t *Task) CostSeconds() float64 {
	if t.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if t.FinishedAt != nil {
		end = *t.FinishedAt
	}
	return end.Sub(*t.StartedAt).Seconds()
}

// taskJSON 用于自定义 JSON 序列化（包含 cost_seconds 计算字段）
type taskJSON struct {
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
	VideoDuration float64      `json:"video_duration,omitempty"`
	CostSeconds   float64      `json:"cost_seconds,omitempty"`
}

// MarshalJSON 自定义序列化，自动填充 cost_seconds
func (t *Task) MarshalJSON() ([]byte, error) {
	return json.Marshal(taskJSON{
		ID:            t.ID,
		State:         t.State,
		Priority:      t.Priority,
		Params:        t.Params,
		Progress:      t.Progress,
		Error:         t.Error,
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
		OutputPath:    t.OutputPath,
		VideoDuration: t.VideoDuration,
		CostSeconds:   math.Round(t.CostSeconds()*10) / 10,
	})
}
