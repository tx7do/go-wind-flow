package flow

import (
	"context"
	"testing"
)

func TestSimpleEvaluator(t *testing.T) {
	ev := NewSimpleEvaluator()
	ctx := context.Background()
	env := Vars{
		"days":   float64(5),
		"amount": 1500,
		"urgent": true,
		"dept":   "IT",
		"status": "A",
		"nested": map[string]any{"level": float64(2)},
		"vip":    false,
	}

	cases := []struct {
		expr string
		want bool
	}{
		{"days > 3", true},
		{"days > 10", false},
		{"days < 10", true},
		{"amount >= 1500", true},
		{"urgent && days > 3", true},
		{"vip || urgent", true},
		{"!vip && urgent", true},
		{"dept == 'IT'", true},
		{`dept != "HR"`, true},
		{"status in ['A', 'B']", true},
		{"status in ['C']", false},
		{"nested.level == 2", true},
		{"days + 1 > 5", true},
		{"amount * 2 == 3000", true},
		{"missing == nil", true},
		{"missing != 'x'", true},
		{"(days > 3 && urgent) || vip", true},
		{"days <= 3", false},
	}

	for _, c := range cases {
		got, err := ev.EvalBool(ctx, c.expr, env)
		if err != nil {
			t.Errorf("EvalBool(%q) error: %v", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("EvalBool(%q) = %v, want %v", c.expr, got, c.want)
		}
	}

	// fail-closed 场景。
	bad := []string{"days >", "urgent && 1", "amount / 0 == 1", "dept + 1 > 2", ""}
	for _, expr := range bad {
		if _, err := ev.EvalBool(ctx, expr, env); err == nil {
			t.Errorf("EvalBool(%q) should fail", expr)
		}
	}
}
