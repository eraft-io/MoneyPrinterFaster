package config

import (
	"os"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Queue    QueueConfig    `toml:"queue"`
	Worker   WorkerConfig   `toml:"worker"`
	LLM      LLMConfig      `toml:"llm"`
	TTS      TTSConfig      `toml:"tts"`
	Subtitle SubtitleConfig `toml:"subtitle"`
	Material MaterialConfig `toml:"material"`
	FFmpeg   FFmpegConfig   `toml:"ffmpeg"`
}

type ServerConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type QueueConfig struct {
	BufferSize int    `toml:"buffer_size"`
	DBPath     string `toml:"db_path"`
}

type WorkerConfig struct {
	MaxWorkers        int           `toml:"max_workers"`
	MinWorkers        int           `toml:"min_workers"`
	HighWaterMark     float64       `toml:"high_water_mark"`
	LowWaterMark      float64       `toml:"low_water_mark"`
	ScaleInterval     time.Duration `toml:"scale_interval"`
	CPUSampleInterval time.Duration `toml:"cpu_sample_interval"`
}

type LLMConfig struct {
	Provider string         `toml:"provider"`
	OpenAI   OpenAIConfig   `toml:"openai"`
	DeepSeek DeepSeekConfig `toml:"deepseek"`
	Ollama   OllamaConfig   `toml:"ollama"`
}

type OpenAIConfig struct {
	APIKey  string `toml:"api_key"`
	BaseURL string `toml:"base_url"`
	Model   string `toml:"model"`
}

type DeepSeekConfig struct {
	APIKey          string `toml:"api_key"`
	BaseURL         string `toml:"base_url"`
	Model           string `toml:"model"`
	EnableThinking  bool   `toml:"enable_thinking"`
	ReasoningEffort string `toml:"reasoning_effort"` // "low" | "medium" | "high"
}

type OllamaConfig struct {
	BaseURL string `toml:"base_url"`
	Model   string `toml:"model"`
}

type TTSConfig struct {
	Provider string          `toml:"provider"`
	Azure    AzureTTSConfig  `toml:"azure"`
	OpenAI   OpenAITTSConfig `toml:"openai"`
}

type AzureTTSConfig struct {
	SpeechKey    string `toml:"speech_key"`
	SpeechRegion string `toml:"speech_region"`
}

type OpenAITTSConfig struct {
	APIKey  string `toml:"api_key"`
	BaseURL string `toml:"base_url"`
	Model   string `toml:"model"`
	Voice   string `toml:"voice"`
}

type SubtitleConfig struct {
	Provider string `toml:"provider"`
}

type MaterialConfig struct {
	Source      string            `toml:"source"`
	LocalDir    string            `toml:"local_dir"`
	Pexels      PexelsConfig      `toml:"pexels"`
	Pixabay     PixabayConfig     `toml:"pixabay"`
	AliyunImage AliyunImageConfig `toml:"aliyun_image"`
}

type PexelsConfig struct {
	APIKeys []string `toml:"api_keys"`
}

type PixabayConfig struct {
	APIKeys []string `toml:"api_keys"`
}

type AliyunImageConfig struct {
	APIKey      string `toml:"api_key"`
	Model       string `toml:"model"`
	ImageCount  int    `toml:"image_count"`  // 每个关键词生成的图片数量
	ClipSeconds int    `toml:"clip_seconds"` // 每张图片展示时长（秒）
}

type FFmpegConfig struct {
	BinaryPath string `toml:"binary_path"`
	VideoCodec string `toml:"video_codec"`
}

var (
	global     *Config
	globalMu   sync.RWMutex
	globalPath string
)

// Load 从指定路径加载 TOML 配置文件
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 配置文件不存在，使用默认值
		globalMu.Lock()
		global = cfg
		globalPath = path
		globalMu.Unlock()
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}

	// 应用默认值填充
	applyDefaults(cfg)
	globalMu.Lock()
	global = cfg
	globalPath = path
	globalMu.Unlock()
	return cfg, nil
}

// Get 获取全局配置（Load 后可调用）
func Get() *Config {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if global == nil {
		return defaultConfig()
	}
	return global
}

// GetPath 获取配置文件路径
func GetPath() string {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalPath
}

// Update 更新全局配置并保存到文件
func Update(newCfg *Config) error {
	applyDefaults(newCfg)
	globalMu.Lock()
	global = newCfg
	path := globalPath
	globalMu.Unlock()

	if path == "" {
		path = "config.toml"
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(newCfg)
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Queue: QueueConfig{
			BufferSize: 100,
			DBPath:     "./data/queue.db",
		},
		Worker: WorkerConfig{
			MaxWorkers:        4,
			MinWorkers:        1,
			HighWaterMark:     0.85,
			LowWaterMark:      0.50,
			ScaleInterval:     5 * time.Second,
			CPUSampleInterval: 2 * time.Second,
		},
		LLM: LLMConfig{
			Provider: "openai",
			OpenAI: OpenAIConfig{
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-4o-mini",
			},
			DeepSeek: DeepSeekConfig{
				BaseURL: "https://api.deepseek.com",
				Model:   "deepseek-v4-pro",
			},
			Ollama: OllamaConfig{
				BaseURL: "http://localhost:11434",
				Model:   "llama3",
			},
		},
		TTS: TTSConfig{
			Provider: "edge",
		},
		Subtitle: SubtitleConfig{
			Provider: "edge",
		},
		Material: MaterialConfig{
			Source: "pexels",
		},
		FFmpeg: FFmpegConfig{
			VideoCodec: "libx264",
		},
	}
}

func applyDefaults(cfg *Config) {
	def := defaultConfig()

	if cfg.Server.Host == "" {
		cfg.Server.Host = def.Server.Host
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = def.Server.Port
	}
	if cfg.Queue.BufferSize == 0 {
		cfg.Queue.BufferSize = def.Queue.BufferSize
	}
	if cfg.Queue.DBPath == "" {
		cfg.Queue.DBPath = def.Queue.DBPath
	}
	if cfg.Worker.MaxWorkers == 0 {
		cfg.Worker.MaxWorkers = def.Worker.MaxWorkers
	}
	if cfg.Worker.MinWorkers == 0 {
		cfg.Worker.MinWorkers = def.Worker.MinWorkers
	}
	if cfg.Worker.HighWaterMark == 0 {
		cfg.Worker.HighWaterMark = def.Worker.HighWaterMark
	}
	if cfg.Worker.LowWaterMark == 0 {
		cfg.Worker.LowWaterMark = def.Worker.LowWaterMark
	}
	if cfg.Worker.ScaleInterval == 0 {
		cfg.Worker.ScaleInterval = def.Worker.ScaleInterval
	}
	if cfg.Worker.CPUSampleInterval == 0 {
		cfg.Worker.CPUSampleInterval = def.Worker.CPUSampleInterval
	}
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = def.LLM.Provider
	}
	if cfg.LLM.OpenAI.BaseURL == "" {
		cfg.LLM.OpenAI.BaseURL = def.LLM.OpenAI.BaseURL
	}
	if cfg.LLM.OpenAI.Model == "" {
		cfg.LLM.OpenAI.Model = def.LLM.OpenAI.Model
	}
	if cfg.LLM.DeepSeek.BaseURL == "" {
		cfg.LLM.DeepSeek.BaseURL = def.LLM.DeepSeek.BaseURL
	}
	if cfg.LLM.DeepSeek.Model == "" {
		cfg.LLM.DeepSeek.Model = def.LLM.DeepSeek.Model
	}
	if cfg.LLM.Ollama.BaseURL == "" {
		cfg.LLM.Ollama.BaseURL = def.LLM.Ollama.BaseURL
	}
	if cfg.LLM.Ollama.Model == "" {
		cfg.LLM.Ollama.Model = def.LLM.Ollama.Model
	}
	if cfg.TTS.Provider == "" {
		cfg.TTS.Provider = def.TTS.Provider
	}
	if cfg.Subtitle.Provider == "" {
		cfg.Subtitle.Provider = def.Subtitle.Provider
	}
	if cfg.Material.Source == "" {
		cfg.Material.Source = def.Material.Source
	}
	if cfg.Material.AliyunImage.Model == "" {
		cfg.Material.AliyunImage.Model = "z-image-turbo"
	}
	if cfg.Material.AliyunImage.ImageCount <= 0 {
		cfg.Material.AliyunImage.ImageCount = 1
	}
	if cfg.Material.AliyunImage.ClipSeconds <= 0 {
		cfg.Material.AliyunImage.ClipSeconds = 5
	}
	if cfg.FFmpeg.VideoCodec == "" {
		cfg.FFmpeg.VideoCodec = def.FFmpeg.VideoCodec
	}
}
