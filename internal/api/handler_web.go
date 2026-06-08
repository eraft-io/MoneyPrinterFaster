package api

import (
	"embed"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
)

//go:embed templates/*.html
var templateFS embed.FS

var (
	templates         *template.Template
	settingsTemplates *template.Template
)

func init() {
	funcMap := template.FuncMap{
		"formatDuration": func(seconds float64) string {
			if seconds <= 0 {
				return "-"
			}
			totalSec := int(math.Round(seconds))
			min := totalSec / 60
			sec := totalSec % 60
			if min > 0 {
				return fmt.Sprintf("%d分%02d秒", min, sec)
			}
			return fmt.Sprintf("%d秒", sec)
		},
		"secsBetween": func(start, end *time.Time) float64 {
			if start == nil || end == nil {
				return 0
			}
			return end.Sub(*start).Seconds()
		},
		"newUUID": func() string {
			return uuid.New().String()
		},
		"truncate": func(s string, n int) string {
			runes := []rune(s)
			if len(runes) <= n {
				return s
			}
			return string(runes[:n]) + "..."
		},
	}
	templates = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))
	// Settings 使用独立模板集（只解析 layout + settings，避免 "content" 定义冲突）
	settingsTemplates = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/settings.html"))
}

// WebHandler HTMX Web 页面 Handler
type WebHandler struct {
	deps *Dependencies
}

func NewWebHandler(deps *Dependencies) *WebHandler {
	return &WebHandler{deps: deps}
}

// IndexPage 首页（创建任务 + 任务列表）
func (h *WebHandler) IndexPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "layout", map[string]any{
		"MaterialSources": h.deps.MaterialSources,
		"ImageConfig":     h.deps.ImageConfig,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// TaskList 任务列表片段（HTMX 局部刷新）
func (h *WebHandler) TaskList(w http.ResponseWriter, r *http.Request) {
	tasks, _, err := h.deps.Queue.List(0, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "task_list", map[string]any{
		"Tasks": tasks,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SettingsPage 配置管理页面
func (h *WebHandler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := settingsTemplates.ExecuteTemplate(w, "layout", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
