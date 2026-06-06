package queue

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"moneyprinterFaster/internal/model"
	"moneyprinterFaster/pkg/utils"
)

// SQLiteStore SQLite 持久化存储
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 创建 SQLite 存储
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := utils.EnsureDir(dir); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}

	// 启用 WAL 模式（SQLite 自身的 WAL，提高并发性能）
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("启用 WAL 模式失败: %w", err)
	}

	// 创建表
	if err := createTables(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("创建表失败: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		state TEXT NOT NULL DEFAULT 'pending',
		priority INTEGER NOT NULL DEFAULT 1,
		params TEXT NOT NULL,
		progress REAL NOT NULL DEFAULT 0,
		error TEXT DEFAULT '',
		created_at TEXT NOT NULL,
		started_at TEXT,
		finished_at TEXT,
		output_path TEXT DEFAULT '',
		video_duration REAL NOT NULL DEFAULT 0,
		current_step TEXT DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks(state);
	CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at DESC);
	`
	_, err := db.Exec(schema)
	return err
}

// Save 保存任务（upsert）
func (s *SQLiteStore) Save(task *model.Task) error {
	paramsJSON, err := json.Marshal(task.Params)
	if err != nil {
		return fmt.Errorf("序列化 params 失败: %w", err)
	}

	var startedAt, finishedAt sql.NullString
	if task.StartedAt != nil {
		startedAt = sql.NullString{String: task.StartedAt.Format(time.RFC3339), Valid: true}
	}
	if task.FinishedAt != nil {
		finishedAt = sql.NullString{String: task.FinishedAt.Format(time.RFC3339), Valid: true}
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO tasks (id, state, priority, params, progress, error, created_at, started_at, finished_at, output_path, video_duration, current_step)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		task.ID,
		string(task.State),
		int(task.Priority),
		string(paramsJSON),
		task.Progress,
		task.Error,
		task.CreatedAt.Format(time.RFC3339),
		startedAt,
		finishedAt,
		task.OutputPath,
		task.VideoDuration,
		"", // current_step 由 UpdateProgress 单独更新
	)
	return err
}

// UpdateState 更新任务状态
func (s *SQLiteStore) UpdateState(taskID string, state model.TaskState, errMsg string, outputPath string, videoDuration float64) error {
	var finishedAt sql.NullString
	if state.IsTerminal() {
		finishedAt = sql.NullString{String: time.Now().Format(time.RFC3339), Valid: true}
	}

	_, err := s.db.Exec(`
		UPDATE tasks SET state = ?, error = ?, finished_at = ?, output_path = ?, video_duration = ?
		WHERE id = ?
	`, string(state), errMsg, finishedAt, outputPath, videoDuration, taskID)
	return err
}

// UpdateProgress 更新进度
func (s *SQLiteStore) UpdateProgress(taskID string, progress float64, step string) error {
	_, err := s.db.Exec(`
		UPDATE tasks SET progress = ?, current_step = ? WHERE id = ?
	`, progress, step, taskID)
	return err
}

// MarkStarted 标记为处理中
func (s *SQLiteStore) MarkStarted(taskID string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE tasks SET state = 'processing', started_at = ?, progress = 0 WHERE id = ?
	`, now, taskID)
	return err
}

// GetPending 获取所有未完成任务（用于恢复）
func (s *SQLiteStore) GetPending() ([]*model.Task, error) {
	rows, err := s.db.Query(`
		SELECT id, state, priority, params, progress, error, created_at, started_at, finished_at, output_path, video_duration
		FROM tasks
		WHERE state IN ('pending', 'processing')
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			continue
		}
		// processing 状态恢复为 pending（可能上次崩溃了）
		task.State = model.StatePending
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// ListAll 列出所有任务（按创建时间降序）
func (s *SQLiteStore) ListAll() ([]*model.Task, error) {
	rows, err := s.db.Query(`
		SELECT id, state, priority, params, progress, error, created_at, started_at, finished_at, output_path, video_duration
		FROM tasks
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// Delete 删除任务
func (s *SQLiteStore) Delete(taskID string) error {
	_, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", taskID)
	return err
}

// Close 关闭数据库
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// scanTask 从行扫描任务数据
func scanTask(rows *sql.Rows) (*model.Task, error) {
	var id, state, paramsJSON, errMsg, createdAtStr string
	var priority int
	var progress, videoDuration float64
	var startedAtStr, finishedAtStr sql.NullString
	var outputPath string

	err := rows.Scan(&id, &state, &priority, &paramsJSON, &progress, &errMsg, &createdAtStr,
		&startedAtStr, &finishedAtStr, &outputPath, &videoDuration)
	if err != nil {
		return nil, err
	}

	var params model.VideoParams
	json.Unmarshal([]byte(paramsJSON), &params)

	createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

	task := &model.Task{
		ID:            id,
		State:         model.TaskState(state),
		Priority:      model.TaskPriority(priority),
		Params:        params,
		Progress:      progress,
		Error:         errMsg,
		CreatedAt:     createdAt,
		OutputPath:    outputPath,
		VideoDuration: videoDuration,
	}

	if startedAtStr.Valid {
		if t, err := time.Parse(time.RFC3339, startedAtStr.String); err == nil {
			task.StartedAt = &t
		}
	}
	if finishedAtStr.Valid {
		if t, err := time.Parse(time.RFC3339, finishedAtStr.String); err == nil {
			task.FinishedAt = &t
		}
	}

	return task, nil
}
