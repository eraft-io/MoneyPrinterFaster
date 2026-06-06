package llm

import (
	"context"
	"fmt"

	"moneyprinterFaster/internal/config"
)

// Provider LLM 服务接口
type Provider interface {
	GenerateScript(ctx context.Context, subject, language string, paragraphs int) (string, error)
	GenerateTerms(ctx context.Context, subject, script string) ([]string, error)
}

// NewProvider 根据配置创建 LLM Provider
func NewProvider(cfg *config.LLMConfig) (Provider, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAI(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model), nil
	case "deepseek":
		return NewDeepSeek(cfg.DeepSeek.APIKey, cfg.DeepSeek.BaseURL, cfg.DeepSeek.Model, cfg.DeepSeek.EnableThinking, cfg.DeepSeek.ReasoningEffort), nil
	case "ollama":
		return NewOllama(cfg.Ollama.BaseURL, cfg.Ollama.Model), nil
	default:
		return nil, fmt.Errorf("不支持的 LLM provider: %s", cfg.Provider)
	}
}
