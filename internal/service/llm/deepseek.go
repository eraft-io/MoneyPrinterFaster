package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// DeepSeek DeepSeek LLM Provider（原生 HTTP 实现，支持 thinking/reasoning）
type DeepSeek struct {
	apiKey          string
	baseURL         string
	model           string
	reasoningEffort string // "low" | "medium" | "high"
	enableThinking  bool
	client          *http.Client
}

// NewDeepSeek 创建 DeepSeek Provider
func NewDeepSeek(apiKey, baseURL, model string, enableThinking bool, reasoningEffort string) *DeepSeek {
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	// 确保 base_url 不以 /v1 结尾（DeepSeek 原生接口直接是 /chat/completions）
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimSuffix(baseURL, "/v1")
	}

	if model == "" {
		model = "deepseek-v4-pro"
	}
	if reasoningEffort == "" {
		reasoningEffort = "medium"
	}

	d := &DeepSeek{
		apiKey:          apiKey,
		baseURL:         baseURL,
		model:           model,
		reasoningEffort: reasoningEffort,
		enableThinking:  enableThinking,
		client:          &http.Client{},
	}

	log.Printf("[DeepSeek] 初始化: base_url=%s, model=%s, thinking=%v, effort=%s, has_key=%v",
		d.baseURL, d.model, d.enableThinking, d.reasoningEffort, apiKey != "")

	return d
}

// deepSeekRequest DeepSeek API 请求体
type deepSeekRequest struct {
	Model           string            `json:"model"`
	Messages        []deepSeekMessage `json:"messages"`
	Stream          bool              `json:"stream"`
	Thinking        *deepSeekThinking `json:"thinking,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
}

type deepSeekMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type deepSeekThinking struct {
	Type string `json:"type"` // "enabled" | "disabled"
}

// deepSeekResponse DeepSeek API 响应体
type deepSeekResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int             `json:"index"`
		Message      deepSeekMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (d *DeepSeek) callAPI(ctx context.Context, messages []deepSeekMessage) (string, error) {
	reqBody := deepSeekRequest{
		Model:    d.model,
		Messages: messages,
		Stream:   false,
	}

	// 启用 thinking 时传入参数
	if d.enableThinking {
		reqBody.Thinking = &deepSeekThinking{Type: "enabled"}
		reqBody.ReasoningEffort = d.reasoningEffort
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := d.baseURL + "/chat/completions"
	log.Printf("[DeepSeek] 请求: POST %s, model=%s, messages=%d", url, d.model, len(messages))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	log.Printf("[DeepSeek] 响应: status=%d, body_len=%d", resp.StatusCode, len(respBody))

	if resp.StatusCode != http.StatusOK {
		log.Printf("[DeepSeek] 错误响应: %s", string(respBody))
		return "", fmt.Errorf("API 返回 %d: %s", resp.StatusCode, string(respBody))
	}

	var dsResp deepSeekResponse
	if err := json.Unmarshal(respBody, &dsResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w (body: %s)", err, string(respBody))
	}

	if dsResp.Error != nil {
		return "", fmt.Errorf("API 错误: %s", dsResp.Error.Message)
	}

	if len(dsResp.Choices) == 0 {
		return "", fmt.Errorf("未返回任何内容")
	}

	msg := dsResp.Choices[0].Message
	log.Printf("[DeepSeek] 完成: tokens(in=%d, out=%d, total=%d), finish=%s",
		dsResp.Usage.PromptTokens, dsResp.Usage.CompletionTokens, dsResp.Usage.TotalTokens,
		dsResp.Choices[0].FinishReason)

	if msg.ReasoningContent != "" {
		log.Printf("[DeepSeek] reasoning_content 长度: %d 字符", len(msg.ReasoningContent))
	}

	return msg.Content, nil
}

func (d *DeepSeek) GenerateScript(ctx context.Context, subject, language string, paragraphs int) (string, error) {
	prompt := fmt.Sprintf(`Generate a video script about "%s".
Requirements:
1. Return as a single string with %d paragraphs separated by newlines.
2. Do not reference this prompt.
3. Get straight to the point, no "welcome to this video" etc.
4. No markdown or formatting, no titles.
5. Only return the raw script content.
6. Do not include "voiceover", "narrator" or similar indicators.
7. Do not mention the prompt or the script itself.
8. Respond in %s language.`, subject, paragraphs, language)

	messages := []deepSeekMessage{
		{Role: "system", Content: "You are a helpful assistant that writes video scripts."},
		{Role: "user", Content: prompt},
	}

	content, err := d.callAPI(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("DeepSeek API 调用失败: %w", err)
	}

	content = cleanThinkBlocks(content)
	return strings.TrimSpace(content), nil
}

func (d *DeepSeek) GenerateTerms(ctx context.Context, subject, script string) ([]string, error) {
	prompt := fmt.Sprintf(`Based on the following video script about "%s", generate 5 search terms for finding stock video footage.
Return ONLY the search terms, one per line, in English. No numbering, no explanations.

Script:
%s`, subject, script)

	messages := []deepSeekMessage{
		{Role: "system", Content: "You are a helpful assistant that generates search keywords."},
		{Role: "user", Content: prompt},
	}

	content, err := d.callAPI(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("DeepSeek API 调用失败: %w", err)
	}

	content = cleanThinkBlocks(content)
	lines := strings.Split(content, "\n")
	var terms []string
	for _, line := range lines {
		term := strings.TrimSpace(line)
		term = strings.TrimLeft(term, "0123456789.-) ")
		if term != "" {
			terms = append(terms, term)
		}
	}

	if len(terms) == 0 {
		return nil, fmt.Errorf("未生成有效的搜索关键词")
	}
	return terms, nil
}
