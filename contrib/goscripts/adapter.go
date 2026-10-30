// Package goscripts 把 go-scripts 接入 go-wind-flow 的 SPI：
//
//   - Runner：实现 flow.ScriptRunner，驱动脚本任务（JS/Lua/Python/Go/...）；
//   - Evaluator：实现 flow.ExpressionEvaluator（复用 go-scripts 的 CEL/Expr 引擎）。
//
// 沙箱约定：脚本环境只注入实例变量与白名单宿主函数；执行经 context
// deadline 控时。生产高并发建议在宿主侧为 Runner 包一层引擎池。
package goscripts

import (
	"context"
	"fmt"
	"sync"

	// 引擎自注册：按需引入所需语言。默认带上 JS 与 Expr；
	// Lua/Python/CEL/Go/WASM 等在宿主侧 blank import 即可启用。
	_ "github.com/tx7do/go-scripts/expr"
	_ "github.com/tx7do/go-scripts/javascript"

	scriptEngine "github.com/tx7do/go-scripts"

	"github.com/tx7do/go-wind-flow/flow"
)

// Runner flow.ScriptRunner 的 go-scripts 实现。
type Runner struct {
	mu      sync.Mutex
	engines map[string]scriptEngine.Engine
}

// New 创建 Runner。langs 省略时按需懒加载。
func New() *Runner {
	return &Runner{engines: map[string]scriptEngine.Engine{}}
}

// Run 执行脚本任务。脚本可通过 return 返回 map[string]any 合并进实例变量。
func (r *Runner) Run(ctx context.Context, spec *flow.ScriptSpec, env flow.Vars) (any, error) {
	if spec == nil || spec.Code == "" {
		return nil, fmt.Errorf("%w: empty script", flow.ErrScript)
	}
	eng, err := r.acquire(spec.Lang)
	if err != nil {
		return nil, fmt.Errorf("%w: acquire engine %q: %v", flow.ErrScript, spec.Lang, err)
	}
	for k, v := range env {
		_ = eng.RegisterGlobal(k, v)
	}
	_ = eng.RegisterGlobal("__env", map[string]any(env))
	return eng.ExecuteString(ctx, "flow-task", spec.Code)
}

// acquire 懒加载并复用每种语言一个引擎实例。
// 注意：单引擎非并发安全，同一 Runner 并发执行需宿主加锁或改用引擎池。
func (r *Runner) acquire(lang string) (scriptEngine.Engine, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.engines == nil {
		r.engines = map[string]scriptEngine.Engine{}
	}
	eng, ok := r.engines[lang]
	if ok && eng.IsInitialized() {
		return eng, nil
	}
	typ := scriptEngine.Type(lang)
	if typ == "" {
		typ = scriptEngine.JavaScriptType
	}
	created, err := scriptEngine.NewScriptEngine(typ)
	if err != nil {
		return nil, err
	}
	if err := created.Init(context.Background()); err != nil {
		return nil, err
	}
	r.engines[lang] = created
	return created, nil
}

// Close 关闭全部引擎。
func (r *Runner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for _, eng := range r.engines {
		if err := eng.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.engines = map[string]scriptEngine.Engine{}
	return firstErr
}

// Evaluator flow.ExpressionEvaluator 的 go-scripts 实现（CEL 或 Expr 引擎）。
type Evaluator struct {
	typ scriptEngine.Type
}

// NewExprEvaluator 用 go-scripts 的 Expr 引擎求值。
func NewExprEvaluator() *Evaluator { return &Evaluator{typ: scriptEngine.ExprType} }

// NewCELEvaluator 用 go-scripts 的 CEL 引擎求值。
func NewCELEvaluator() *Evaluator { return &Evaluator{typ: scriptEngine.CELType} }

func (e *Evaluator) EvalBool(ctx context.Context, expression string, env flow.Vars) (bool, error) {
	if expression == "" {
		return false, fmt.Errorf("%w: empty expression", flow.ErrExpression)
	}
	eng, err := scriptEngine.NewScriptEngine(e.typ)
	if err != nil {
		return false, fmt.Errorf("%w: new engine: %v", flow.ErrExpression, err)
	}
	defer func() { _ = eng.Close() }()
	if err := eng.Init(ctx); err != nil {
		return false, fmt.Errorf("%w: init: %v", flow.ErrExpression, err)
	}
	for k, v := range env {
		_ = eng.RegisterGlobal(k, v)
	}
	output, err := eng.ExecuteString(ctx, "flow-condition", expression)
	if err != nil {
		return false, fmt.Errorf("%w: eval: %v", flow.ErrExpression, err)
	}
	b, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("%w: result is %T, want bool", flow.ErrExpression, output)
	}
	return b, nil
}
