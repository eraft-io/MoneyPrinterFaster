package api

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"moneyprinterFaster/internal/pipeline"
	"moneyprinterFaster/internal/queue"
	"moneyprinterFaster/internal/worker"
)

// Dependencies API 依赖注入
type Dependencies struct {
	Queue         queue.Queue
	Pool          *worker.WorkerPool
	StaticFS      fs.FS                // 静态资源文件系统（从 main 传入）
	LLM           pipeline.LLMProvider // LLM 服务（用于文案预览）
	MaterialReady bool                 // Material Provider 是否已配置
	MaterialError string               // Material Provider 配置错误信息
}

// NewRouter 创建 HTTP 路由
func NewRouter(deps *Dependencies) http.Handler {
	r := chi.NewRouter()

	// 中间件
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		h := NewTaskHandler(deps)
		r.Post("/tasks", h.CreateTask)
		r.Get("/tasks", h.ListTasks)
		r.Get("/tasks/{id}", h.GetTask)
		r.Delete("/tasks/{id}", h.CancelTask)
		r.Get("/tasks/{id}/download", h.DownloadVideo)
		r.Get("/status", h.GetStatus)

		// LLM 相关
		llmH := NewLLMHandler(deps)
		r.Post("/llm/generate-script", llmH.GenerateScript)
	})

	// WebSocket
	r.Get("/ws", NewWSHandler(deps).Handle)

	// Web (HTMX) — 在 Task 7 中实现模板渲染
	// 这里先挂载静态资源
	if deps.StaticFS != nil {
		staticSub, err := fs.Sub(deps.StaticFS, "static")
		if err == nil {
			r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
		}
	}

	// Web (HTMX)
	web := NewWebHandler(deps)
	r.Get("/", web.IndexPage)
	r.Get("/tasks/list", web.TaskList)

	return r
}
