package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"moneyprinterFaster/internal/model"
)

// MemoryQueue 基于 Channel 的内存队列 + WAL 持久化
type MemoryQueue struct {
	mu       sync.RWMutex
	tasks    map[string]*model.Task
	ch       chan *model.Task
	wal      *WAL
	subs     []chan model.ProgressEvent
	subsMu   sync.RWMutex
	closed   bool
}

// NewMemoryQueue 创建内存队列实例
func NewMemoryQueue(bufSize int, walDir string) (*MemoryQueue, error) {
	wal, err := NewWAL(walDir)
	if err != nil {
		return nil, fmt.Errorf("初始化 WAL 失败: %w", err)
	}

	q := &MemoryQueue{
		tasks: make(map[string]*model.Task),
		ch:    make(chan *model.Task, bufSize),
		wal:   wal,
	}

	// 从 WAL 恢复未完成任务
	if err := q.recoverFromWAL(); err != nil {
		// 恢复失败不阻塞启动，记录日志即可
		fmt.Printf("警告: WAL 恢复失败: %v\n", err)
	}

	return q, nil
}

// recoverFromWAL 从 WAL 恢复未完成任务到队列
func (q *MemoryQueue) recoverFromWAL() error {
	pending, err := q.wal.Recover()
	if err != nil {
		return err
	}

	// 按创建时间排序
	sort.Slice(pending, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, pending[i].CreatedAt)
		tj, _ := time.Parse(time.RFC3339, pending[j].CreatedAt)
		return ti.Before(tj)
	})

	for _, wt := range pending {
		var params model.VideoParams
		json.Unmarshal(wt.Params, &params)

		task := &model.Task{
			ID:       wt.ID,
			State:    model.StatePending,
			Priority: model.TaskPriority(wt.Priority),
			Params:   params,
		}
		if t, err := time.Parse(time.RFC3339, wt.CreatedAt); err == nil {
			task.CreatedAt = t
		} else {
			task.CreatedAt = time.Now()
		}

		q.tasks[task.ID] = task

		select {
		case q.ch <- task:
		default:
			// channel 满，任务留在索引中等候
		}
	}

	if len(pending) > 0 {
		fmt.Printf("从 WAL 恢复了 %d 个未完成任务\n", len(pending))
	}
	return nil
}

// Submit 提交任务到队列
func (q *MemoryQueue) Submit(ctx context.Context, params model.VideoParams, priority model.TaskPriority) (string, error) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return "", fmt.Errorf("队列已关闭")
	}
	q.mu.Unlock()

	params.Defaults()

	task := &model.Task{
		ID:        uuid.New().String(),
		State:     model.StatePending,
		Priority:  priority,
		Params:    params,
		CreatedAt: time.Now(),
	}

	// 1. 写 WAL
	paramsJSON, _ := json.Marshal(params)
	walEntry := WALEntry{
		Type:   WALTypeSubmit,
		TaskID: task.ID,
		Task: &WALTaskData{
			ID:        task.ID,
			State:     string(task.State),
			Priority:  int(task.Priority),
			Params:    paramsJSON,
			CreatedAt: task.CreatedAt.Format(time.RFC3339),
		},
	}
	if err := q.wal.Append(walEntry); err != nil {
		return "", fmt.Errorf("WAL 写入失败: %w", err)
	}

	// 2. 入内存索引
	q.mu.Lock()
	q.tasks[task.ID] = task
	q.mu.Unlock()

	// 3. 非阻塞入 channel
	select {
	case q.ch <- task:
	default:
		// channel 满，任务已在索引中，dequeue 会兜底扫描
	}

	q.notify(model.NewProgressEvent(task.ID, model.StatePending, 0, "", "任务已提交"))
	return task.ID, nil
}

// Dequeue 从队列取出一个任务（阻塞等待）
func (q *MemoryQueue) Dequeue(ctx context.Context) (*model.Task, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case task := <-q.ch:
		if task == nil {
			return nil, fmt.Errorf("队列已关闭")
		}
		// 检查是否已被取消
		q.mu.RLock()
		latest, ok := q.tasks[task.ID]
		q.mu.RUnlock()
		if !ok || latest.State.IsTerminal() {
			// 任务已被取消或已完成，跳过
			return q.Dequeue(ctx) // 递归取下一个
		}
		return task, nil
	}
}

// Cancel 取消任务
func (q *MemoryQueue) Cancel(taskID string) error {
	q.mu.Lock()
	task, ok := q.tasks[taskID]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("任务不存在: %s", taskID)
	}

	if task.State.IsTerminal() {
		q.mu.Unlock()
		return fmt.Errorf("任务已终态，无法取消: %s (state=%s)", taskID, task.State)
	}

	// 调用取消函数
	if task.CancelFunc != nil {
		task.CancelFunc()
	}
	task.MarkCancelled()
	q.mu.Unlock()

	// 写 WAL
	q.wal.Append(WALEntry{Type: WALTypeCancel, TaskID: taskID})
	q.notify(model.NewProgressEvent(taskID, model.StateCancelled, task.Progress, "", "任务已取消"))
	return nil
}

// Get 获取任务
func (q *MemoryQueue) Get(taskID string) (*model.Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	task, ok := q.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	return task, nil
}

// List 列出任务（分页）
func (q *MemoryQueue) List(offset, limit int) ([]*model.Task, int, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// 收集所有任务并按创建时间降序排列
	all := make([]*model.Task, 0, len(q.tasks))
	for _, task := range q.tasks {
		all = append(all, task)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	total := len(all)
	if offset >= total {
		return []*model.Task{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

// UpdateProgress 更新任务进度
func (q *MemoryQueue) UpdateProgress(taskID string, progress float64, step string) error {
	q.mu.Lock()
	task, ok := q.tasks[taskID]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task.Progress = progress
	q.mu.Unlock()

	q.notify(model.NewProgressEvent(taskID, model.StateProcessing, progress, step, ""))
	return nil
}

// SetState 设置任务状态
func (q *MemoryQueue) SetState(taskID string, state model.TaskState, errMsg string) error {
	q.mu.Lock()
	task, ok := q.tasks[taskID]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("任务不存在: %s", taskID)
	}

	switch state {
	case model.StateProcessing:
		task.MarkProcessing()
	case model.StateCompleted:
		task.MarkCompleted(task.OutputPath)
	case model.StateFailed:
		if errMsg != "" {
			task.MarkFailed(fmt.Errorf("%s", errMsg))
		} else {
			task.MarkFailed(nil)
		}
	case model.StateCancelled:
		task.MarkCancelled()
	}
	q.mu.Unlock()

	// 终态写 WAL
	if state.IsTerminal() {
		var walType WALEntryType
		switch state {
		case model.StateCompleted:
			walType = WALTypeComplete
		case model.StateFailed:
			walType = WALTypeFail
		case model.StateCancelled:
			walType = WALTypeCancel
		}
		q.wal.Append(WALEntry{Type: walType, TaskID: taskID})
	}

	evt := model.NewProgressEvent(taskID, state, task.Progress, "", errMsg)
	q.notify(evt)
	return nil
}

// Subscribe 订阅任务事件
func (q *MemoryQueue) Subscribe() <-chan model.ProgressEvent {
	q.subsMu.Lock()
	defer q.subsMu.Unlock()

	ch := make(chan model.ProgressEvent, 64)
	q.subs = append(q.subs, ch)
	return ch
}

// Unsubscribe 取消订阅
func (q *MemoryQueue) Unsubscribe(ch <-chan model.ProgressEvent) {
	q.subsMu.Lock()
	defer q.subsMu.Unlock()

	for i, sub := range q.subs {
		if sub == ch {
			q.subs = append(q.subs[:i], q.subs[i+1:]...)
			close(sub)
			return
		}
	}
}

// notify 通知所有订阅者
func (q *MemoryQueue) notify(evt model.ProgressEvent) {
	q.subsMu.RLock()
	defer q.subsMu.RUnlock()

	for _, ch := range q.subs {
		select {
		case ch <- evt:
		default:
			// 订阅者消费慢，丢弃
		}
	}
}

// Close 关闭队列
func (q *MemoryQueue) Close() error {
	q.mu.Lock()
	q.closed = true
	close(q.ch)
	q.mu.Unlock()

	q.subsMu.Lock()
	for _, ch := range q.subs {
		close(ch)
	}
	q.subs = nil
	q.subsMu.Unlock()

	return q.wal.Close()
}
