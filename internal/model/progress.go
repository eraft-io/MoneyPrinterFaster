package model

import "time"

// ProgressEvent 进度事件（用于 WebSocket 推送）
type ProgressEvent struct {
	TaskID    string    `json:"task_id"`
	State     TaskState `json:"state"`
	Progress  float64   `json:"progress"`  // 0-100
	Step      string    `json:"step"`      // 当前步骤名称
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
}

// NewProgressEvent 创建进度事件
func NewProgressEvent(taskID string, state TaskState, progress float64, step, message string) ProgressEvent {
	return ProgressEvent{
		TaskID:    taskID,
		State:     state,
		Progress:  progress,
		Step:      step,
		Message:   message,
		Timestamp: time.Now(),
	}
}

// NewErrorEvent 创建错误事件
func NewErrorEvent(taskID string, err error) ProgressEvent {
	return ProgressEvent{
		TaskID:    taskID,
		State:     StateFailed,
		Error:     err.Error(),
		Timestamp: time.Now(),
	}
}

// WorkerStatus Worker Pool 状态（用于监控面板）
type WorkerStatus struct {
	ActiveWorkers int     `json:"active_workers"`
	TargetWorkers int     `json:"target_workers"`
	CPUUsage      float64 `json:"cpu_usage"`
	QueueLength   int     `json:"queue_length"`
	NumCPU        int     `json:"num_cpu"`
}
