package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"moneyprinterFaster/internal/model"
)

// TaskHandler 任务 API Handler
type TaskHandler struct {
	deps *Dependencies
}

func NewTaskHandler(deps *Dependencies) *TaskHandler {
	return &TaskHandler{deps: deps}
}

// CreateTask 创建任务
// POST /api/v1/tasks
// 支持 JSON 和 form-urlencoded 两种格式
func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject          string  `json:"subject"`
		Script           string  `json:"script,omitempty"`
		Language         string  `json:"language"`
		ParagraphNumber  int     `json:"paragraph_number"`
		Aspect           string  `json:"aspect"`
		ConcatMode       string  `json:"concat_mode"`
		TransitionMode   string  `json:"transition_mode"`
		ClipDuration     int     `json:"clip_duration"`
		VideoCount       int     `json:"video_count"`
		VoiceName        string  `json:"voice_name"`
		VoiceRate        float64 `json:"voice_rate"`
		SubtitleEnabled  bool    `json:"subtitle_enabled"`
		SubtitleFont     string  `json:"subtitle_font"`
		SubtitleSize     int     `json:"subtitle_size"`
		SubtitleColor    string  `json:"subtitle_color"`
		SubtitlePosition string  `json:"subtitle_position"`
		SubtitleStroke   bool    `json:"subtitle_stroke"`
		BGMEnabled       bool    `json:"bgm_enabled"`
		BGMFile          string  `json:"bgm_file,omitempty"`
		BGMVolume        float64 `json:"bgm_volume"`
		Priority         int     `json:"priority"`
	}

	contentType := r.Header.Get("Content-Type")
	log.Printf("[Task] 收到创建任务请求: content_type=%s", contentType)

	if strings.HasPrefix(contentType, "application/json") {
		// JSON 格式
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("[Task] JSON 解析失败: %v", err)
			writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
			return
		}
	} else {
		// Form-urlencoded 格式（HTMX 表单提交）
		if err := r.ParseForm(); err != nil {
			log.Printf("[Task] Form 解析失败: %v", err)
			writeError(w, http.StatusBadRequest, "表单解析失败: "+err.Error())
			return
		}
		req.Subject = r.FormValue("subject")
		req.Script = r.FormValue("script")
		req.Language = r.FormValue("language")
		req.Aspect = r.FormValue("aspect")
		req.ConcatMode = r.FormValue("concat_mode")
		req.TransitionMode = r.FormValue("transition_mode")
		req.VoiceName = r.FormValue("voice_name")
		req.SubtitleFont = r.FormValue("subtitle_font")
		req.SubtitleColor = r.FormValue("subtitle_color")
		req.SubtitlePosition = r.FormValue("subtitle_position")
		req.BGMFile = r.FormValue("bgm_file")
		req.ParagraphNumber, _ = strconv.Atoi(r.FormValue("paragraph_number"))
		req.ClipDuration, _ = strconv.Atoi(r.FormValue("clip_duration"))
		req.VideoCount, _ = strconv.Atoi(r.FormValue("video_count"))
		req.SubtitleSize, _ = strconv.Atoi(r.FormValue("subtitle_size"))
		req.Priority, _ = strconv.Atoi(r.FormValue("priority"))
		req.VoiceRate, _ = strconv.ParseFloat(r.FormValue("voice_rate"), 64)
		req.BGMVolume, _ = strconv.ParseFloat(r.FormValue("bgm_volume"), 64)
		// Checkbox 值处理
		req.SubtitleEnabled = r.FormValue("subtitle_enabled") == "on"
		req.SubtitleStroke = r.FormValue("subtitle_stroke") == "on"
		req.BGMEnabled = r.FormValue("bgm_enabled") == "on"
	}

	log.Printf("[Task] 任务参数: subject=%q, script_len=%d, language=%s, paragraphs=%d, aspect=%s",
		req.Subject, len(req.Script), req.Language, req.ParagraphNumber, req.Aspect)

	if req.Subject == "" && req.Script == "" {
		writeError(w, http.StatusBadRequest, "必须提供 subject 或 script")
		return
	}

	// 检查素材服务是否已配置
	if !h.deps.MaterialReady {
		errMsg := "素材服务未配置，无法生成视频。"
		if h.deps.MaterialError != "" {
			errMsg += "原因: " + h.deps.MaterialError + "\n"
		}
		errMsg += "请访问 https://www.pexels.com/api/ 免费申请 API Key，然后在 config.toml 的 [material.pexels] 中填写 api_keys"
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	params := model.VideoParams{
		Subject:          req.Subject,
		Script:           req.Script,
		Language:         req.Language,
		ParagraphNumber:  req.ParagraphNumber,
		Aspect:           model.VideoAspect(req.Aspect),
		ConcatMode:       model.VideoConcatMode(req.ConcatMode),
		TransitionMode:   model.VideoTransitionMode(req.TransitionMode),
		ClipDuration:     req.ClipDuration,
		VideoCount:       req.VideoCount,
		VoiceName:        req.VoiceName,
		VoiceRate:        req.VoiceRate,
		SubtitleEnabled:  req.SubtitleEnabled,
		SubtitleFont:     req.SubtitleFont,
		SubtitleSize:     req.SubtitleSize,
		SubtitleColor:    req.SubtitleColor,
		SubtitlePosition: req.SubtitlePosition,
		SubtitleStroke:   req.SubtitleStroke,
		BGMEnabled:       req.BGMEnabled,
		BGMFile:          req.BGMFile,
		BGMVolume:        req.BGMVolume,
	}

	taskID, err := h.deps.Queue.Submit(r.Context(), params, model.TaskPriority(req.Priority))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "提交任务失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"task_id": taskID,
		"status":  "submitted",
	})
}

// ListTasks 列出任务
// GET /api/v1/tasks?offset=0&limit=20
func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}

	tasks, total, err := h.deps.Queue.List(offset, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询任务失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tasks":  tasks,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	})
}

// GetTask 获取任务详情
// GET /api/v1/tasks/{id}
func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")

	task, err := h.deps.Queue.Get(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "任务不存在: "+taskID)
		return
	}

	writeJSON(w, http.StatusOK, task)
}

// CancelTask 取消任务
// DELETE /api/v1/tasks/{id}
func (h *TaskHandler) CancelTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")

	if err := h.deps.Queue.Cancel(taskID); err != nil {
		writeError(w, http.StatusBadRequest, "取消失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"task_id": taskID,
		"status":  "cancelled",
	})
}

// DownloadVideo 下载视频文件
// GET /api/v1/tasks/{id}/download
func (h *TaskHandler) DownloadVideo(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")

	task, err := h.deps.Queue.Get(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "任务不存在: "+taskID)
		return
	}

	if task.State != model.StateCompleted {
		writeError(w, http.StatusBadRequest, "任务未完成，无法下载")
		return
	}

	if task.OutputPath == "" {
		writeError(w, http.StatusNotFound, "输出文件不存在")
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+task.ID+".mp4")
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, task.OutputPath)
}

// GetStatus 获取系统状态
// GET /api/v1/status
func (h *TaskHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status := h.deps.Pool.Status()
	writeJSON(w, http.StatusOK, status)
}

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError 写入错误响应
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
