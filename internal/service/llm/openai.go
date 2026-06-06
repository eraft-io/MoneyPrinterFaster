package llm

import (
	"context"
	"fmt"
	"log"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAI OpenAI 兼容 LLM Provider
type OpenAI struct {
	client *openai.Client
	model  string
}

// NewOpenAI 创建 OpenAI Provider
func NewOpenAI(apiKey, baseURL, model string) *OpenAI {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	log.Printf("[OpenAI Client] 初始化: base_url=%s, model=%s, has_key=%v", cfg.BaseURL, model, apiKey != "")
	return &OpenAI{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
	}
}

func (o *OpenAI) GenerateScript(ctx context.Context, subject, language string, paragraphs int) (string, error) {
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

	resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: o.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("OpenAI API 调用失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("OpenAI 未返回任何内容")
	}

	content := resp.Choices[0].Message.Content
	// 清理思考标签（reasoning 模型）
	content = cleanThinkBlocks(content)
	return strings.ReplaceAll(strings.TrimSpace(content), "\n", ""), nil
}

func (o *OpenAI) GenerateTerms(ctx context.Context, subject, script string) ([]string, error) {
	prompt := fmt.Sprintf(`Based on the following video script about "%s", generate 5 search terms for finding stock video footage.
Return ONLY the search terms, one per line, in English. No numbering, no explanations.

Script:
%s`, subject, script)

	resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: o.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("OpenAI API 调用失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI 未返回任何内容")
	}

	content := cleanThinkBlocks(resp.Choices[0].Message.Content)
	lines := strings.Split(content, "\n")
	var terms []string
	for _, line := range lines {
		term := strings.TrimSpace(line)
		term = strings.TrimLeft(term, "0123456789.-) ") // 去除编号
		if term != "" {
			terms = append(terms, term)
		}
	}

	if len(terms) == 0 {
		return nil, fmt.Errorf("未生成有效的搜索关键词")
	}
	return terms, nil
}

// cleanThinkBlocks 清理 reasoning 模型的 think 标签
func cleanThinkBlocks(content string) string {
	openTag := "<think>"
	closeTag := "</think>"
	for {
		start := strings.Index(content, openTag)
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], closeTag)
		if end == -1 {
			content = content[:start]
			break
		}
		content = content[:start] + content[start+end+len(closeTag):]
	}
	return strings.TrimSpace(content)
}
