package goscripts

import (
	"context"
	"testing"

	scriptEngine "github.com/tx7do/go-scripts"

	"github.com/tx7do/go-wind-flow/flow"
)

func TestRunnerJavaScript(t *testing.T) {
	r := New()
	defer func() { _ = r.Close() }()

	out, err := r.Run(context.Background(), &flow.ScriptSpec{
		Lang: string(scriptEngine.JavaScriptType),
		Code: `({ total: amount * days, level: amount > 1000 ? 'high' : 'low' })`,
	}, flow.Vars{"amount": float64(2000), "days": float64(3)})
	if err != nil {
		t.Fatalf("run js: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("result type %T, want map", out)
	}
	if m["level"] != "high" {
		t.Fatalf("level = %v, want high", m["level"])
	}
}

func TestEvaluatorExpr(t *testing.T) {
	ev := NewExprEvaluator()
	ok, err := ev.EvalBool(context.Background(), "amount > 1000 && urgent",
		flow.Vars{"amount": float64(2000), "urgent": true})
	if err != nil || !ok {
		t.Fatalf("EvalBool = %v, %v", ok, err)
	}
}
