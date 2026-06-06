package pipeline

import (
	"context"

	"moneyprinterFaster/internal/model"
)

// StepResult Pipeline 步骤执行结果
type StepResult struct {
	Data map[string]any // 步骤产出数据，传递给下游步骤
}

// Step Pipeline 步骤接口
type Step interface {
	Name() string
	Execute(ctx context.Context, task *model.Task, prevData map[string]any) (*StepResult, error)
}
