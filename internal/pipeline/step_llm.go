package pipeline

import (
	"context"
	"fmt"

	"moneyprinterFaster/internal/model"
)

// LLMProvider LLM 服务接口
type LLMProvider interface {
	GenerateScript(ctx context.Context, subject, language string, paragraphs int) (string, error)
	GenerateTerms(ctx context.Context, subject, script string) ([]string, error)
}

// StepLLM LLM 文案生成步骤
type StepLLM struct {
	provider LLMProvider
}

func NewStepLLM(provider LLMProvider) *StepLLM {
	return &StepLLM{provider: provider}
}

func (s *StepLLM) Name() string { return "llm" }

func (s *StepLLM) Execute(ctx context.Context, task *model.Task, prevData map[string]any) (*StepResult, error) {
	params := task.Params

	if s.provider == nil {
		return nil, fmt.Errorf("LLM 服务未配置，无法执行")
	}

	// 使用自定义文案或自动生成
	script := params.Script
	if script == "" {
		var err error
		script, err = s.provider.GenerateScript(ctx, params.Subject, params.Language, params.ParagraphNumber)
		if err != nil {
			return nil, fmt.Errorf("生成文案失败: %w", err)
		}
		// 回写到 task，以便持久化到 SQLite
		task.Params.Script = script
	}

	// 生成搜索关键词
	terms, err := s.provider.GenerateTerms(ctx, params.Subject, script)
	if err != nil {
		return nil, fmt.Errorf("生成关键词失败: %w", err)
	}

	return &StepResult{
		Data: map[string]any{
			"script": script,
			"terms":  terms,
		},
	}, nil
}
