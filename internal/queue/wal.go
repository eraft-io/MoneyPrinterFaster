package queue

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"moneyprinterFaster/pkg/utils"
)

// WALEntry WAL 日志条目类型
type WALEntryType string

const (
	WALTypeSubmit   WALEntryType = "SUBMIT"
	WALTypeComplete WALEntryType = "COMPLETE"
	WALTypeFail     WALEntryType = "FAIL"
	WALTypeCancel   WALEntryType = "CANCEL"
)

// WALEntry WAL 日志条目
type WALEntry struct {
	Type   WALEntryType   `json:"type"`
	TaskID string         `json:"task_id"`
	Task   *WALTaskData   `json:"task,omitempty"` // 仅 SUBMIT 时携带
}

// WALTaskData WAL 中存储的任务数据（去掉运行时字段）
type WALTaskData struct {
	ID        string                `json:"id"`
	State     string                `json:"state"`
	Priority  int                   `json:"priority"`
	Params    json.RawMessage       `json:"params"`
	CreatedAt string                `json:"created_at"`
}

// WAL Write-Ahead Log
type WAL struct {
	mu      sync.Mutex
	dir     string
	file    *os.File
	writer  *bufio.Writer
}

// NewWAL 创建 WAL 实例
func NewWAL(dir string) (*WAL, error) {
	if err := utils.EnsureDir(dir); err != nil {
		return nil, fmt.Errorf("创建 WAL 目录失败: %w", err)
	}

	logPath := filepath.Join(dir, "queue.wal")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开 WAL 文件失败: %w", err)
	}

	return &WAL{
		dir:    dir,
		file:   file,
		writer: bufio.NewWriter(file),
	}, nil
}

// Append 追加写入 WAL
func (w *WAL) Append(entry WALEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化 WAL 条目失败: %w", err)
	}

	if _, err := w.writer.Write(data); err != nil {
		return fmt.Errorf("写入 WAL 失败: %w", err)
	}
	if _, err := w.writer.WriteString("\n"); err != nil {
		return fmt.Errorf("写入 WAL 换行失败: %w", err)
	}
	return w.writer.Flush()
}

// Recover 从 WAL 恢复未完成的任务
func (w *WAL) Recover() ([]WALTaskData, error) {
	logPath := filepath.Join(w.dir, "queue.wal")
	if !utils.FileExists(logPath) {
		return nil, nil
	}

	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("打开 WAL 文件失败: %w", err)
	}
	defer file.Close()

	// 记录所有提交的任务
	submitted := make(map[string]WALTaskData)
	// 记录已终态的任务
	terminal := make(map[string]bool)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 支持大行

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry WALEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // 跳过损坏的条目
		}

		switch entry.Type {
		case WALTypeSubmit:
			if entry.Task != nil {
				submitted[entry.TaskID] = *entry.Task
			}
		case WALTypeComplete, WALTypeFail, WALTypeCancel:
			terminal[entry.TaskID] = true
		}
	}

	// 返回未终态的任务
	var pending []WALTaskData
	for id, task := range submitted {
		if !terminal[id] {
			pending = append(pending, task)
		}
	}

	return pending, scanner.Err()
}

// Compact 压缩 WAL（移除已终态任务的条目）
func (w *WAL) Compact() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	logPath := filepath.Join(w.dir, "queue.wal")
	tmpPath := filepath.Join(w.dir, "queue.wal.tmp")

	// 关闭当前 writer
	w.writer.Flush()
	w.file.Close()

	// 读取并过滤
	file, err := os.Open(logPath)
	if err != nil {
		return err
	}

	terminal := make(map[string]bool)
	var entries []string

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry WALEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case WALTypeComplete, WALTypeFail, WALTypeCancel:
			terminal[entry.TaskID] = true
		default:
			entries = append(entries, line)
		}
	}
	file.Close()

	// 写回只保留未完成的任务
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	tmpWriter := bufio.NewWriter(tmpFile)

	for _, line := range entries {
		var entry WALEntry
		json.Unmarshal([]byte(line), &entry)
		if !terminal[entry.TaskID] {
			tmpWriter.WriteString(line)
			tmpWriter.WriteString("\n")
		}
	}
	tmpWriter.Flush()
	tmpFile.Close()

	// 原子替换
	return os.Rename(tmpPath, logPath)
}

// Close 关闭 WAL
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer != nil {
		w.writer.Flush()
	}
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
