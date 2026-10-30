package goscripts

import (
	"context"
	"errors"
	"testing"

	scriptEngine "github.com/tx7do/go-scripts"

	"github.com/tx7do/go-wind-flow/flow"
)

// TestRunnerErrors 空脚本、未知语言、语法错误全部 fail-closed 为 ErrScript。
func TestRunnerErrors(t *testing.T) {
	r := New()
	defer func() { _ = r.Close() }()
	ctx := context.Background()

	if _, err := r.Run(ctx, nil, nil); !errors.Is(err, flow.ErrScript) {
		t.Errorf("nil spec err = %v, want ErrScript", err)
	}
	if _, err := r.Run(ctx, &flow.ScriptSpec{Code: ""}, nil); !errors.Is(err, flow.ErrScript) {
		t.Errorf("empty script err = %v, want ErrScript", err)
	}
	if _, err := r.Run(ctx, &flow.ScriptSpec{Lang: "no-such-lang", Code: "1"}, nil); !errors.Is(err, flow.ErrScript) {
		t.Errorf("unknown lang err = %v, want ErrScript", err)
	}
	if _, err := r.Run(ctx, &flow.ScriptSpec{
		Lang: string(scriptEngine.JavaScriptType), Code: "this is not js (((",
	}, nil); err == nil {
		t.Error("syntax error should surface")
	}
}

// TestRunnerReuse 同语言复用同一引擎实例（二次调用走已初始化分支）。
func TestRunnerReuse(t *testing.T) {
	r := New()
	defer func() { _ = r.Close() }()
	ctx := context.Background()
	spec := &flow.ScriptSpec{Lang: string(scriptEngine.JavaScriptType), Code: "1 + 1"}
	for i := 0; i < 3; i++ {
		if _, err := r.Run(ctx, spec, flow.Vars{"x": float64(i)}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

// TestEvaluatorCEL CEL 求值（引擎不可用时须返回 ErrExpression 而非 panic）。
func TestEvaluatorCEL(t *testing.T) {
	ev := NewCELEvaluator()
	ok, err := ev.EvalBool(context.Background(), "amount > 1000 && urgent",
		flow.Vars{"amount": float64(2000), "urgent": true})
	if err != nil {
		// CEL 引擎未随当前依赖注册：要求仍是可识别的哨兵错误。
		if !errors.Is(err, flow.ErrExpression) {
			t.Fatalf("CEL unavailable error should wrap ErrExpression, got %v", err)
		}
		return
	}
	if !ok {
		t.Fatal("CEL EvalBool = false, want true")
	}
}

// TestEvaluatorErrors 空表达式、求值错误、非布尔结果。
func TestEvaluatorErrors(t *testing.T) {
	ev := NewExprEvaluator()
	ctx := context.Background()

	if _, err := ev.EvalBool(ctx, "", flow.Vars{}); !errors.Is(err, flow.ErrExpression) {
		t.Errorf("empty expr err = %v, want ErrExpression", err)
	}
	// 语法错误。
	if _, err := ev.EvalBool(ctx, "amount >", flow.Vars{"amount": float64(1)}); !errors.Is(err, flow.ErrExpression) {
		t.Errorf("syntax err = %v, want ErrExpression", err)
	}
	// 结果非布尔 → ErrExpression（fail-closed）。
	if _, err := ev.EvalBool(ctx, "amount + 1", flow.Vars{"amount": float64(1)}); !errors.Is(err, flow.ErrExpression) {
		t.Errorf("non-bool result err = %v, want ErrExpression", err)
	}
}
