package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"moneyprinterFaster/internal/config"
)

// ConfigHandler 配置管理 Handler
type ConfigHandler struct {
	deps *Dependencies
}

func NewConfigHandler(deps *Dependencies) *ConfigHandler {
	return &ConfigHandler{deps: deps}
}

// GetConfig 获取当前配置
// GET /api/v1/config
func (h *ConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()

	// 转换为前端友好格式（API Key 脱敏）
	resp := configResponse{
		Server: serverResp{
			Host: cfg.Server.Host,
			Port: cfg.Server.Port,
		},
		Queue: queueResp{
			BufferSize: cfg.Queue.BufferSize,
			DBPath:     cfg.Queue.DBPath,
		},
		Worker: workerResp{
			MaxWorkers:    cfg.Worker.MaxWorkers,
			MinWorkers:    cfg.Worker.MinWorkers,
			HighWaterMark: cfg.Worker.HighWaterMark,
			LowWaterMark:  cfg.Worker.LowWaterMark,
		},
		LLM: llmResp{
			Provider: cfg.LLM.Provider,
			OpenAI: openaiResp{
				APIKey:  maskKey(cfg.LLM.OpenAI.APIKey),
				BaseURL: cfg.LLM.OpenAI.BaseURL,
				Model:   cfg.LLM.OpenAI.Model,
			},
			DeepSeek: deepseekResp{
				APIKey:          maskKey(cfg.LLM.DeepSeek.APIKey),
				BaseURL:         cfg.LLM.DeepSeek.BaseURL,
				Model:           cfg.LLM.DeepSeek.Model,
				EnableThinking:  cfg.LLM.DeepSeek.EnableThinking,
				ReasoningEffort: cfg.LLM.DeepSeek.ReasoningEffort,
			},
			Ollama: ollamaResp{
				BaseURL: cfg.LLM.Ollama.BaseURL,
				Model:   cfg.LLM.Ollama.Model,
			},
		},
		TTS: ttsResp{
			Provider: cfg.TTS.Provider,
			Azure: azureResp{
				SpeechKey:    maskKey(cfg.TTS.Azure.SpeechKey),
				SpeechRegion: cfg.TTS.Azure.SpeechRegion,
			},
			OpenAI: openaiTTSResp{
				APIKey:  maskKey(cfg.TTS.OpenAI.APIKey),
				BaseURL: cfg.TTS.OpenAI.BaseURL,
				Model:   cfg.TTS.OpenAI.Model,
				Voice:   cfg.TTS.OpenAI.Voice,
			},
		},
		Subtitle: subtitleResp{
			Provider: cfg.Subtitle.Provider,
		},
		Material: materialResp{
			Source:   cfg.Material.Source,
			LocalDir: cfg.Material.LocalDir,
			Pexels: pexelsResp{
				APIKeys: maskKeys(cfg.Material.Pexels.APIKeys),
			},
			Pixabay: pixabayResp{
				APIKeys: maskKeys(cfg.Material.Pixabay.APIKeys),
			},
			AliyunImage: aliyunImageResp{
				APIKey:      maskKey(cfg.Material.AliyunImage.APIKey),
				Model:       cfg.Material.AliyunImage.Model,
				ImageCount:  cfg.Material.AliyunImage.ImageCount,
				ClipSeconds: cfg.Material.AliyunImage.ClipSeconds,
			},
		},
		FFmpeg: ffmpegResp{
			BinaryPath: cfg.FFmpeg.BinaryPath,
			VideoCodec: cfg.FFmpeg.VideoCodec,
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
}

// SaveConfig 保存配置
// POST /api/v1/config
func (h *ConfigHandler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	var req configRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "解析请求失败: " + err.Error()})
		return
	}

	// 获取当前配置作为基础
	cfg := *config.Get()

	// 应用用户提交的修改（保留脱敏的 Key 不覆盖）
	applyConfigUpdate(&cfg, &req)

	// 保存配置
	if err := config.Update(&cfg); err != nil {
		log.Printf("[Config] 保存配置失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存配置失败: " + err.Error()})
		return
	}

	log.Printf("[Config] 配置已保存: %s", h.deps.ConfigPath)
	writeJSON(w, http.StatusOK, map[string]string{
		"message": "配置已保存，部分配置需要重启服务后生效",
		"path":    h.deps.ConfigPath,
	})
}

// applyConfigUpdate 将请求中的非空字段应用到配置
func applyConfigUpdate(cfg *config.Config, req *configRequest) {
	// Server
	if req.Server.Host != "" {
		cfg.Server.Host = req.Server.Host
	}
	if req.Server.Port > 0 {
		cfg.Server.Port = req.Server.Port
	}

	// Queue
	if req.Queue.DBPath != "" {
		cfg.Queue.DBPath = req.Queue.DBPath
	}
	if req.Queue.BufferSize > 0 {
		cfg.Queue.BufferSize = req.Queue.BufferSize
	}

	// Worker
	if req.Worker.MaxWorkers > 0 {
		cfg.Worker.MaxWorkers = req.Worker.MaxWorkers
	}
	if req.Worker.MinWorkers > 0 {
		cfg.Worker.MinWorkers = req.Worker.MinWorkers
	}
	if req.Worker.HighWaterMark > 0 {
		cfg.Worker.HighWaterMark = req.Worker.HighWaterMark
	}
	if req.Worker.LowWaterMark > 0 {
		cfg.Worker.LowWaterMark = req.Worker.LowWaterMark
	}

	// LLM
	if req.LLM.Provider != "" {
		cfg.LLM.Provider = req.LLM.Provider
	}
	if req.LLM.OpenAI.BaseURL != "" {
		cfg.LLM.OpenAI.BaseURL = req.LLM.OpenAI.BaseURL
	}
	if req.LLM.OpenAI.Model != "" {
		cfg.LLM.OpenAI.Model = req.LLM.OpenAI.Model
	}
	// API Key 只有非脱敏值才覆盖
	if req.LLM.OpenAI.APIKey != "" && !isMasked(req.LLM.OpenAI.APIKey) {
		cfg.LLM.OpenAI.APIKey = req.LLM.OpenAI.APIKey
	}
	if req.LLM.DeepSeek.BaseURL != "" {
		cfg.LLM.DeepSeek.BaseURL = req.LLM.DeepSeek.BaseURL
	}
	if req.LLM.DeepSeek.Model != "" {
		cfg.LLM.DeepSeek.Model = req.LLM.DeepSeek.Model
	}
	if req.LLM.DeepSeek.APIKey != "" && !isMasked(req.LLM.DeepSeek.APIKey) {
		cfg.LLM.DeepSeek.APIKey = req.LLM.DeepSeek.APIKey
	}
	cfg.LLM.DeepSeek.EnableThinking = req.LLM.DeepSeek.EnableThinking
	if req.LLM.DeepSeek.ReasoningEffort != "" {
		cfg.LLM.DeepSeek.ReasoningEffort = req.LLM.DeepSeek.ReasoningEffort
	}
	if req.LLM.Ollama.BaseURL != "" {
		cfg.LLM.Ollama.BaseURL = req.LLM.Ollama.BaseURL
	}
	if req.LLM.Ollama.Model != "" {
		cfg.LLM.Ollama.Model = req.LLM.Ollama.Model
	}

	// TTS
	if req.TTS.Provider != "" {
		cfg.TTS.Provider = req.TTS.Provider
	}
	if req.TTS.Azure.SpeechKey != "" && !isMasked(req.TTS.Azure.SpeechKey) {
		cfg.TTS.Azure.SpeechKey = req.TTS.Azure.SpeechKey
	}
	if req.TTS.Azure.SpeechRegion != "" {
		cfg.TTS.Azure.SpeechRegion = req.TTS.Azure.SpeechRegion
	}
	if req.TTS.OpenAI.APIKey != "" && !isMasked(req.TTS.OpenAI.APIKey) {
		cfg.TTS.OpenAI.APIKey = req.TTS.OpenAI.APIKey
	}
	if req.TTS.OpenAI.BaseURL != "" {
		cfg.TTS.OpenAI.BaseURL = req.TTS.OpenAI.BaseURL
	}
	if req.TTS.OpenAI.Model != "" {
		cfg.TTS.OpenAI.Model = req.TTS.OpenAI.Model
	}
	if req.TTS.OpenAI.Voice != "" {
		cfg.TTS.OpenAI.Voice = req.TTS.OpenAI.Voice
	}

	// Subtitle
	if req.Subtitle.Provider != "" {
		cfg.Subtitle.Provider = req.Subtitle.Provider
	}

	// Material
	if req.Material.Source != "" {
		cfg.Material.Source = req.Material.Source
	}
	if req.Material.LocalDir != "" {
		cfg.Material.LocalDir = req.Material.LocalDir
	}
	// Pexels API Keys - 处理逗号分隔字符串
	if req.Material.Pexels.APIKeys != "" && !isMasked(req.Material.Pexels.APIKeys) {
		cfg.Material.Pexels.APIKeys = splitKeys(req.Material.Pexels.APIKeys)
	}
	if req.Material.Pixabay.APIKeys != "" && !isMasked(req.Material.Pixabay.APIKeys) {
		cfg.Material.Pixabay.APIKeys = splitKeys(req.Material.Pixabay.APIKeys)
	}
	if req.Material.AliyunImage.APIKey != "" && !isMasked(req.Material.AliyunImage.APIKey) {
		cfg.Material.AliyunImage.APIKey = req.Material.AliyunImage.APIKey
	}
	if req.Material.AliyunImage.Model != "" {
		cfg.Material.AliyunImage.Model = req.Material.AliyunImage.Model
	}
	if req.Material.AliyunImage.ImageCount > 0 {
		cfg.Material.AliyunImage.ImageCount = req.Material.AliyunImage.ImageCount
	}
	if req.Material.AliyunImage.ClipSeconds > 0 {
		cfg.Material.AliyunImage.ClipSeconds = req.Material.AliyunImage.ClipSeconds
	}

	// FFmpeg
	if req.FFmpeg.BinaryPath != "" {
		cfg.FFmpeg.BinaryPath = req.FFmpeg.BinaryPath
	}
	if req.FFmpeg.VideoCodec != "" {
		cfg.FFmpeg.VideoCodec = req.FFmpeg.VideoCodec
	}
}

// maskKey 脱敏 API Key
func maskKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// maskKeys 脱敏 API Key 列表
func maskKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return maskKey(keys[0])
}

// isMasked 判断是否为脱敏值
func isMasked(s string) bool {
	return strings.Contains(s, "****")
}

// splitKeys 将逗号分隔的字符串切分为切片
func splitKeys(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// Response types
type configResponse struct {
	Server   serverResp   `json:"server"`
	Queue    queueResp    `json:"queue"`
	Worker   workerResp   `json:"worker"`
	LLM      llmResp      `json:"llm"`
	TTS      ttsResp      `json:"tts"`
	Subtitle subtitleResp `json:"subtitle"`
	Material materialResp `json:"material"`
	FFmpeg   ffmpegResp   `json:"ffmpeg"`
}

type serverResp struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type queueResp struct {
	BufferSize int    `json:"buffer_size"`
	DBPath     string `json:"db_path"`
}

type workerResp struct {
	MaxWorkers    int     `json:"max_workers"`
	MinWorkers    int     `json:"min_workers"`
	HighWaterMark float64 `json:"high_water_mark"`
	LowWaterMark  float64 `json:"low_water_mark"`
}

type llmResp struct {
	Provider string       `json:"provider"`
	OpenAI   openaiResp   `json:"openai"`
	DeepSeek deepseekResp `json:"deepseek"`
	Ollama   ollamaResp   `json:"ollama"`
}

type openaiResp struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type deepseekResp struct {
	APIKey          string `json:"api_key"`
	BaseURL         string `json:"base_url"`
	Model           string `json:"model"`
	EnableThinking  bool   `json:"enable_thinking"`
	ReasoningEffort string `json:"reasoning_effort"`
}

type ollamaResp struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type ttsResp struct {
	Provider string        `json:"provider"`
	Azure    azureResp     `json:"azure"`
	OpenAI   openaiTTSResp `json:"openai"`
}

type azureResp struct {
	SpeechKey    string `json:"speech_key"`
	SpeechRegion string `json:"speech_region"`
}

type openaiTTSResp struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Voice   string `json:"voice"`
}

type subtitleResp struct {
	Provider string `json:"provider"`
}

type materialResp struct {
	Source      string          `json:"source"`
	LocalDir    string          `json:"local_dir"`
	Pexels      pexelsResp      `json:"pexels"`
	Pixabay     pixabayResp     `json:"pixabay"`
	AliyunImage aliyunImageResp `json:"aliyun_image"`
}

type pexelsResp struct {
	APIKeys string `json:"api_keys"`
}

type pixabayResp struct {
	APIKeys string `json:"api_keys"`
}

type aliyunImageResp struct {
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	ImageCount  int    `json:"image_count"`
	ClipSeconds int    `json:"clip_seconds"`
}

type ffmpegResp struct {
	BinaryPath string `json:"binary_path"`
	VideoCodec string `json:"video_codec"`
}

// Request types
type configRequest struct {
	Server   serverReq   `json:"server"`
	Queue    queueReq    `json:"queue"`
	Worker   workerReq   `json:"worker"`
	LLM      llmReq      `json:"llm"`
	TTS      ttsReq      `json:"tts"`
	Subtitle subtitleReq `json:"subtitle"`
	Material materialReq `json:"material"`
	FFmpeg   ffmpegReq   `json:"ffmpeg"`
}

type serverReq struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type queueReq struct {
	BufferSize int    `json:"buffer_size"`
	DBPath     string `json:"db_path"`
}

type workerReq struct {
	MaxWorkers    int     `json:"max_workers"`
	MinWorkers    int     `json:"min_workers"`
	HighWaterMark float64 `json:"high_water_mark"`
	LowWaterMark  float64 `json:"low_water_mark"`
}

type llmReq struct {
	Provider string       `json:"provider"`
	OpenAI   openaiReq    `json:"openai"`
	DeepSeek deepseekReq  `json:"deepseek"`
	Ollama   ollamaReq    `json:"ollama"`
}

type openaiReq struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type deepseekReq struct {
	APIKey          string `json:"api_key"`
	BaseURL         string `json:"base_url"`
	Model           string `json:"model"`
	EnableThinking  bool   `json:"enable_thinking"`
	ReasoningEffort string `json:"reasoning_effort"`
}

type ollamaReq struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type ttsReq struct {
	Provider string        `json:"provider"`
	Azure    azureReq      `json:"azure"`
	OpenAI   openaiTTSReq  `json:"openai"`
}

type azureReq struct {
	SpeechKey    string `json:"speech_key"`
	SpeechRegion string `json:"speech_region"`
}

type openaiTTSReq struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Voice   string `json:"voice"`
}

type subtitleReq struct {
	Provider string `json:"provider"`
}

type materialReq struct {
	Source      string          `json:"source"`
	LocalDir    string          `json:"local_dir"`
	Pexels      pexelsReq       `json:"pexels"`
	Pixabay     pixabayReq      `json:"pixabay"`
	AliyunImage aliyunImageReq  `json:"aliyun_image"`
}

type pexelsReq struct {
	APIKeys string `json:"api_keys"`
}

type pixabayReq struct {
	APIKeys string `json:"api_keys"`
}

type aliyunImageReq struct {
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	ImageCount  int    `json:"image_count"`
	ClipSeconds int    `json:"clip_seconds"`
}

type ffmpegReq struct {
	BinaryPath string `json:"binary_path"`
	VideoCodec string `json:"video_codec"`
}
