package queue

import (
	"context"

	"moneyprinterFaster/internal/model"
)

// Queue 任务队列接口
type Queue interface {
	// Submit 提交任务到队列，返回任务 ID
	Submit(ctx context.Context, params model.VideoParams, priority model.TaskPriority) (string, error)
	// Cancel 取消任务
	Cancel(taskID string) error
	// Get 获取任务状态
	Get(taskID string) (*model.Task, error)
	// List 列出任务（分页）
	List(offset, limit int) ([]*model.Task, int, error)
	// Dequeue 从队列取出一个任务（阻塞），仅 worker 内部使用
	Dequeue(ctx context.Context) (*model.Task, error)
	// UpdateProgress 更新任务进度
	UpdateProgress(taskID string, progress float64, step string) error
	// SetState 设置任务状态
	SetState(taskID string, state model.TaskState, errMsg string) error
	// Subscribe 订阅任务状态变更事件（用于 WebSocket 推送）
	Subscribe() <-chan model.ProgressEvent
	// FindExistingTask 查找已存在的相同主题+文案任务（用于去重）
	FindExistingTask(subject, script string) *model.Task
	// Close 关闭队列
	Close() error
}
