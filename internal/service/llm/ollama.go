package llm

// Ollama Ollama 本地 LLM Provider（兼容 OpenAI 接口）
type Ollama struct {
	*OpenAI
}

// NewOllama 创建 Ollama Provider
func NewOllama(baseURL, model string) *Ollama {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	// Ollama 的 OpenAI 兼容端点是 /v1
	baseURL = baseURL + "/v1"
	if model == "" {
		model = "llama3"
	}
	// Ollama 不需要 API key，但 OpenAI SDK 要求非空
	return &Ollama{
		OpenAI: NewOpenAI("ollama", baseURL, model),
	}
}
