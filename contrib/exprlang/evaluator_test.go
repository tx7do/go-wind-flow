package exprlang

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

func TestEvalBool(t *testing.T) {
	ev := New()
	ctx := context.Background()
	env := flow.Vars{"days": float64(5), "urgent": true, "dept": "IT"}

	ok, err := ev.EvalBool(ctx, "days > 3 && urgent", env)
	if err != nil || !ok {
		t.Fatalf("EvalBool = %v, %v", ok, err)
	}
	ok, err = ev.EvalBool(ctx, "${days <= 3}", env) // UEL 包裹兼容
	if err != nil || ok {
		t.Fatalf("EvalBool UEL = %v, %v", ok, err)
	}
	if _, err := ev.EvalBool(ctx, "days >=", env); err == nil {
		t.Fatal("syntax error should fail")
	}
}
