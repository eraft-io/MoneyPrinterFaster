package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"moneyprinterFaster/internal/model"
	"moneyprinterFaster/internal/queue"
)

// PoolConfig Worker Pool 配置
type PoolConfig struct {
	MaxWorkers    int           // 最大 worker 数
	MinWorkers    int           // 最小 worker 数
	HighWaterMark float64       // CPU 高水位（超过则缩减）
	LowWaterMark  float64       // CPU 低水位（低于则扩容）
	ScaleInterval time.Duration // 扩缩容检查间隔
}

// Validate 校验配置合法性
func (c *PoolConfig) Validate() {
	if c.MaxWorkers <= 0 {
		c.MaxWorkers = 4
	}
	if c.MinWorkers <= 0 {
		c.MinWorkers = 1
	}
	if c.MinWorkers > c.MaxWorkers {
		c.MinWorkers = c.MaxWorkers
	}
	if c.HighWaterMark <= 0 || c.HighWaterMark > 1 {
		c.HighWaterMark = 0.85
	}
	if c.LowWaterMark <= 0 || c.LowWaterMark >= c.HighWaterMark {
		c.LowWaterMark = 0.50
	}
	if c.ScaleInterval <= 0 {
		c.ScaleInterval = 5 * time.Second
	}
}

// WorkerPool CPU 自适应 Worker Pool
type WorkerPool struct {
	cfg      PoolConfig
	queue    queue.Queue
	monitor  *CPUMonitor
	executor *Executor

	mu          sync.Mutex
	activeCount int                      // 当前活跃 worker 数
	targetCount int                      // 目标 worker 数
	running     map[string]bool          // worker goroutine 标识
	stopSignals map[string]chan struct{} // worker 停止信号（缩容时关闭）
}

// NewPool 创建 Worker Pool
func NewPool(cfg PoolConfig, q queue.Queue, monitor *CPUMonitor) *WorkerPool {
	cfg.Validate()
	return &WorkerPool{
		cfg:         cfg,
		queue:       q,
		monitor:     monitor,
		executor:    NewExecutor(q),
		activeCount: cfg.MinWorkers,
		targetCount: cfg.MinWorkers,
		running:     make(map[string]bool),
		stopSignals: make(map[string]chan struct{}),
	}
}

// GetExecutor 获取执行器（用于注入 Pipeline）
func (p *WorkerPool) GetExecutor() *Executor {
	return p.executor
}

// Start 启动 Pool
func (p *WorkerPool) Start(ctx context.Context) {
	// 启动 CPU monitor
	p.monitor.Start(ctx)

	// 拉起初始 worker
	for i := 0; i < p.cfg.MinWorkers; i++ {
		p.spawnWorker(ctx)
	}

	// 启动扩缩容控制器
	go p.scaleLoop(ctx)

	log.Printf("WorkerPool 启动完成: min=%d, max=%d", p.cfg.MinWorkers, p.cfg.MaxWorkers)
}

// Status 返回当前 Pool 状态
func (p *WorkerPool) Status() model.WorkerStatus {
	p.mu.Lock()
	active := p.activeCount
	target := p.targetCount
	p.mu.Unlock()

	return model.WorkerStatus{
		ActiveWorkers: active,
		TargetWorkers: target,
		CPUUsage:      p.monitor.Usage(),
		NumCPU:        p.monitor.NumCPU(),
	}
}

// scaleLoop 扩缩容控制循环
func (p *WorkerPool) scaleLoop(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.ScaleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.adjustWorkers(ctx)
		}
	}
}

// adjustWorkers 根据 CPU 负载调整 worker 数量
func (p *WorkerPool) adjustWorkers(ctx context.Context) {
	cpuUsage := p.monitor.Usage()

	p.mu.Lock()
	current := p.activeCount
	p.mu.Unlock()

	target := current

	switch {
	case cpuUsage > p.cfg.HighWaterMark:
		// CPU 过载，缩减 worker（保留至少 MinWorkers）
		target = max(current-1, p.cfg.MinWorkers)

	case cpuUsage < p.cfg.LowWaterMark:
		// CPU 空闲，扩容（不超过 MaxWorkers）
		// 扩容步长：距上限的一半，避免震荡
		headroom := p.cfg.MaxWorkers - current
		if headroom > 0 {
			step := max(headroom/2, 1)
			target = min(current+step, p.cfg.MaxWorkers)
		}
	}

	// 中水位区间保持不变（迟滞带）

	if target == current {
		return
	}

	p.mu.Lock()
	p.targetCount = target
	p.mu.Unlock()

	if target > current {
		// 扩容：启动新 worker
		for i := 0; i < target-current; i++ {
			p.spawnWorker(ctx)
		}
		log.Printf("WorkerPool 扩容: %d → %d (CPU: %.1f%%)", current, target, cpuUsage*100)
	} else {
		// 缩容：发送停止信号给空闲 worker（不影响正在执行的任务）
		p.mu.Lock()
		toRemove := current - target
		removed := 0
		for id, stopCh := range p.stopSignals {
			if removed >= toRemove {
				break
			}
			select {
			case stopCh <- struct{}{}: // 非阻塞发送停止信号
				delete(p.stopSignals, id)
				removed++
			default:
				// worker 正在执行任务，跳过（让它完成当前任务后自行退出）
			}
		}
		p.mu.Unlock()
		if removed > 0 {
			log.Printf("WorkerPool 缩容: %d → %d (CPU: %.1f%%)", current, current-removed, cpuUsage*100)
		}
	}
}

// spawnWorker 启动一个 worker goroutine
func (p *WorkerPool) spawnWorker(parentCtx context.Context) {
	workerID := uuid.New().String()
	stopCh := make(chan struct{}, 1) // 缓冲 1，确保信号不丢失

	p.mu.Lock()
	p.activeCount++
	p.running[workerID] = true
	p.stopSignals[workerID] = stopCh
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			p.activeCount--
			delete(p.running, workerID)
			delete(p.stopSignals, workerID)
			p.mu.Unlock()
		}()

		for {
			// 先检查停止信号（不阻塞）
			select {
			case <-stopCh:
				log.Printf("[Worker %s] 收到停止信号，退出循环", workerID[:8])
				return
			case <-parentCtx.Done():
				return
			default:
			}

			task, err := p.queue.Dequeue(parentCtx)
			if err != nil {
				if parentCtx.Err() != nil {
					return
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// 再次检查停止信号（可能在 Dequeue 等待期间收到）
			select {
			case <-stopCh:
				log.Printf("[Worker %s] 收到停止信号（Dequeue 后），退出循环", workerID[:8])
				// 任务已取出但未执行，重新入队
				go func() {
					p.queue.Submit(parentCtx, task.Params, task.Priority)
				}()
				return
			default:
			}

			// 执行任务（阻塞），使用 parentCtx 而非 worker ctx
			// 这样缩容不会影响正在执行的任务
			p.executor.Execute(parentCtx, task)
		}
	}()
}

// Shutdown 优雅关闭（等待所有 worker 完成当前任务）
func (p *WorkerPool) Shutdown(timeout time.Duration) {
	p.mu.Lock()
	// 通知所有 worker 停止接受新任务
	for _, stopCh := range p.stopSignals {
		select {
		case stopCh <- struct{}{}:
		default:
		}
	}
	p.mu.Unlock()

	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			log.Println("WorkerPool 关闭超时，强制退出")
			return
		case <-ticker.C:
			p.mu.Lock()
			count := p.activeCount
			p.mu.Unlock()
			if count == 0 {
				fmt.Println("WorkerPool 所有 worker 已退出")
				return
			}
		}
	}
}
