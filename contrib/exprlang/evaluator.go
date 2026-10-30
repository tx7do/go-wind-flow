// Package exprlang 用 expr-lang 实现 flow.ExpressionEvaluator，
// 与 go-wind-oa / go-wind-im 现有网关条件求值行为保持一致。
//
// 用法：
//
//	eng := flow.New(flow.WithStore(memory.New()), flow.WithEvaluator(exprlang.New()))
package exprlang

import (
	"context"
	"fmt"

	"github.com/expr-lang/expr"

	"github.com/tx7do/go-wind-flow/flow"
)

// Evaluator flow.ExpressionEvaluator 的 expr-lang 实现。
type Evaluator struct {
	opts []expr.Option
}

// New 创建求值器。可传入额外编译选项（默认已锁定 Env 为变量 map、
// 要求布尔结果——与核心 SimpleEvaluator 同语义）。
func New(opts ...expr.Option) *Evaluator {
	return &Evaluator{opts: opts}
}

func (e *Evaluator) EvalBool(_ context.Context, expression string, env flow.Vars) (bool, error) {
	expression = trim(expression)
	if expression == "" {
		return false, fmt.Errorf("%w: empty expression", flow.ErrExpression)
	}

	envMap := map[string]any(env)
	opts := append([]expr.Option{
		expr.Env(envMap),
		expr.AsBool(),
		// 安全约束：禁止任意函数调用与全局访问。
		expr.DisableAllBuiltins(),
	}, e.opts...)

	program, err := expr.Compile(expression, opts...)
	if err != nil {
		return false, fmt.Errorf("%w: compile: %v", flow.ErrExpression, err)
	}
	output, err := expr.Run(program, envMap)
	if err != nil {
		return false, fmt.Errorf("%w: run: %v", flow.ErrExpression, err)
	}
	b, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("%w: result is %T, want bool", flow.ErrExpression, output)
	}
	return b, nil
}

func trim(s string) string {
	// 兼容 UEL 包裹。
	s = trimSpace(s)
	if len(s) > 3 && s[0] == '$' && s[1] == '{' && s[len(s)-1] == '}' {
		s = trimSpace(s[2 : len(s)-1])
	}
	return s
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
