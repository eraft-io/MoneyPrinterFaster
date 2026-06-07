package queue

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"moneyprinterFaster/internal/model"
)

// MemoryQueue 基于 Channel 的内存队列 + SQLite 持久化
type MemoryQueue struct {
	mu     sync.RWMutex
	tasks  map[string]*model.Task
	ch     chan *model.Task
	store  *SQLiteStore
	subs   []chan model.ProgressEvent
	subsMu sync.RWMutex
	closed bool
}

// NewMemoryQueue 创建内存队列实例
func NewMemoryQueue(bufSize int, dbPath string) (*MemoryQueue, error) {
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("初始化 SQLite 失败: %w", err)
	}

	q := &MemoryQueue{
		tasks: make(map[string]*model.Task),
		ch:    make(chan *model.Task, bufSize),
		store: store,
	}

	// 从 SQLite 恢复未完成任务
	if err := q.recoverFromStore(); err != nil {
		fmt.Printf("警告: SQLite 恢复失败: %v\n", err)
	}

	return q, nil
}

// recoverFromStore 从 SQLite 恢复所有任务到内存（历史任务用于展示，未完成任务重新入队）
func (q *MemoryQueue) recoverFromStore() error {
	allTasks, err := q.store.ListAll()
	if err != nil {
		return err
	}

	// 全部加载到内存索引
	for _, task := range allTasks {
		q.tasks[task.ID] = task
	}

	// 只有未完成的任务才重新入队执行
	pendingCount := 0
	for _, task := range allTasks {
		if task.State == model.StatePending {
			pendingCount++
			select {
			case q.ch <- task:
			default:
				// channel 满，任务留在索引中等候
			}
		}
	}

	if len(allTasks) > 0 {
		fmt.Printf("从 SQLite 恢复了 %d 个任务 (%d 个未完成，重新入队)\n", len(allTasks), pendingCount)
	}
	return nil
}

// Submit 提交任务到队列（内置主题+文案去重，原子操作）
func (q *MemoryQueue) Submit(ctx context.Context, params model.VideoParams, priority model.TaskPriority) (string, error) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return "", fmt.Errorf("队列已关闭")
	}

	params.Defaults()

	// 去重检查：在锁内查 SQLite，防止并发竞态
	if existing, _ := q.store.FindBySubjectAndScript(params.Subject, params.Script); existing != nil {
		q.mu.Unlock()
		return existing.ID, nil
	}

	task := &model.Task{
		ID:        uuid.New().String(),
		State:     model.StatePending,
		Priority:  priority,
		Params:    params,
		CreatedAt: time.Now(),
	}

	// 1. 写 SQLite（锁内完成，保证原子性）
	if err := q.store.Save(task); err != nil {
		q.mu.Unlock()
		return "", fmt.Errorf("SQLite 写入失败: %w", err)
	}

	// 2. 入内存索引
	q.tasks[task.ID] = task
	q.mu.Unlock()

	// 3. 非阻塞入 channel（满则跳过，Dequeue 兜底扫描会捞到）
	select {
	case q.ch <- task:
	default:
		log.Printf("[Queue] channel 满，任务 %s 未入队，等待兜底扫描", task.ID[:8])
	}

	q.notify(model.NewProgressEvent(task.ID, model.StatePending, 0, "", "任务已提交"))
	return task.ID, nil
}

// Dequeue 从队列取出一个任务（阻塞等待，带兜底扫描）
func (q *MemoryQueue) Dequeue(ctx context.Context) (*model.Task, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case task := <-q.ch:
			if task == nil {
				return nil, fmt.Errorf("队列已关闭")
			}
			q.mu.RLock()
			latest, ok := q.tasks[task.ID]
			q.mu.RUnlock()
			if !ok || latest.State.IsTerminal() {
				continue // 已终态，跳过继续取下一个
			}
			return task, nil
		case <-time.After(2 * time.Second):
			// channel 2 秒无任务，兜底扫描内存索引中的 pending 任务
			task, err := q.scanPending()
			if err != nil {
				return nil, err
			}
			if task != nil {
				return task, nil
			}
			// 无 pending 任务，继续循环等待
		}
	}
}

// scanPending 兜底扫描：从内存索引中查找 pending 任务
func (q *MemoryQueue) scanPending() (*model.Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var oldest *model.Task
	for _, task := range q.tasks {
		if task.State == model.StatePending {
			if oldest == nil || task.CreatedAt.Before(oldest.CreatedAt) {
				oldest = task
			}
		}
	}
	if oldest != nil {
		log.Printf("[Queue] 兜底扫描命中 pending 任务: %s", oldest.ID[:8])
	}
	return oldest, nil
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

	// 更新 SQLite
	q.store.UpdateState(taskID, model.StateCancelled, "", "", 0, task.Params.Script)
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

	// 更新 SQLite
	q.store.UpdateProgress(taskID, progress, step)
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

	// 更新 SQLite
	q.store.UpdateState(taskID, state, task.Error, task.OutputPath, task.VideoDuration, task.Params.Script)

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

// FindExistingTask 查找已存在的相同主题+文案任务（非取消状态）
func (q *MemoryQueue) FindExistingTask(subject, script string) *model.Task {
	task, _ := q.store.FindBySubjectAndScript(subject, script)
	return task
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

	return q.store.Close()
}
