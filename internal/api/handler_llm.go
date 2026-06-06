package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// LLMHandler LLM 相关 API
type LLMHandler struct {
	deps *Dependencies
}

func NewLLMHandler(deps *Dependencies) *LLMHandler {
	return &LLMHandler{deps: deps}
}

// GenerateScript AI 生成文案预览
// POST /api/v1/llm/generate-script
func (h *LLMHandler) GenerateScript(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject         string `json:"subject"`
		Language        string `json:"language"`
		ParagraphNumber int    `json:"paragraph_number"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}

	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "必须提供视频主题")
		return
	}

	if req.Language == "" {
		req.Language = "zh-CN"
	}
	if req.ParagraphNumber <= 0 {
		req.ParagraphNumber = 5
	}

	log.Printf("[LLM] 收到文案生成请求: subject=%q, language=%s, paragraphs=%d", req.Subject, req.Language, req.ParagraphNumber)

	// 调用 LLM 生成文案
	llmProvider := h.deps.LLM
	if llmProvider == nil {
		log.Printf("[LLM] 错误: LLM Provider 为 nil，未配置")
		writeError(w, http.StatusServiceUnavailable, "LLM 服务未配置，请检查 config.toml")
		return
	}

	// 设置 60 秒超时
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	log.Printf("[LLM] 开始调用模型生成文案...")
	start := time.Now()

	script, err := llmProvider.GenerateScript(ctx, req.Subject, req.Language, req.ParagraphNumber)
	elapsed := time.Since(start)

	if err != nil {
		log.Printf("[LLM] 生成文案失败 (耗时 %s): %v", elapsed, err)
		writeError(w, http.StatusInternalServerError, "生成文案失败: "+err.Error())
		return
	}

	log.Printf("[LLM] 文案生成成功 (耗时 %s, 长度 %d 字符)", elapsed, len(script))

	writeJSON(w, http.StatusOK, map[string]string{
		"script": script,
	})
}
