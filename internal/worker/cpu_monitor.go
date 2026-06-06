package worker

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// CPUMonitor CPU 负载监控器
type CPUMonitor struct {
	mu       sync.RWMutex
	current  float64       // 当前 CPU 使用率 (0.0 ~ 1.0)
	interval time.Duration // 采样间隔
	numCPU   int           // 逻辑 CPU 核数
}

// NewCPUMonitor 创建 CPU 监控器
func NewCPUMonitor(interval time.Duration) *CPUMonitor {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &CPUMonitor{
		interval: interval,
		numCPU:   runtime.NumCPU(),
	}
}

// Start 启动后台 CPU 采样 goroutine
func (m *CPUMonitor) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				usage := m.sampleCPU()
				m.mu.Lock()
				m.current = usage
				m.mu.Unlock()
			}
		}
	}()
}

// sampleCPU 采样当前 CPU 使用率
func (m *CPUMonitor) sampleCPU() float64 {
	// 使用 gopsutil 获取系统级 CPU 使用率
	// interval=0 表示使用上次采样结果，首次可能返回 0
	percent, err := cpu.Percent(time.Second, false)
	if err != nil || len(percent) == 0 {
		return 0
	}
	return percent[0] / 100.0 // 转为 0.0 ~ 1.0
}

// Usage 获取当前 CPU 使用率
func (m *CPUMonitor) Usage() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// NumCPU 获取逻辑 CPU 核数
func (m *CPUMonitor) NumCPU() int {
	return m.numCPU
}
