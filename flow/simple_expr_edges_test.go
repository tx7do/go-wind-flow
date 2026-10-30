package flow

import (
	"context"
	"strings"
	"testing"
)

// TestSimpleEvaluatorStringOps 字符串比较/拼接与集合成员、整型宽度。
func TestSimpleEvaluatorStringOps(t *testing.T) {
	ev := NewSimpleEvaluator()
	ctx := context.Background()
	env := Vars{
		"name":  "bob",
		"other": "alice",
		"tags":  []any{"x", "y"},
		"i":     int(3),
		"i8":    int8(8),
		"i16":   int16(16),
		"i32":   int32(32),
		"i64":   int64(64),
		"u":     uint(1),
		"u8":    uint8(8),
		"u16":   uint16(16),
		"u32":   uint32(32),
		"u64":   uint64(64),
		"f32":   float32(1.5),
	}

	ok := []string{
		"name > other", // 字典序
		"other < name",
		"name <= name",
		"name >= 'bob'",
		"name + '!' == 'bob!'", // 字符串拼接
		"i8 + i16 == 24",       // 各整型宽度读入
		"i32 + i64 == 96",
		"u8 + u16 == 24",
		"u32 + u64 == 96",
		"u + i == 4",
		"f32 == 1.5",
		"10 % 3 == 1",
		"-i == -3",
		"'x' in ['x', 'y']", // 数组字面量成员
		"i > 0 && name == 'bob'",
	}
	for _, expr := range ok {
		got, err := ev.EvalBool(ctx, expr, env)
		if err != nil {
			t.Errorf("EvalBool(%q) error: %v", expr, err)
			continue
		}
		if !got {
			t.Errorf("EvalBool(%q) = false, want true", expr)
		}
	}

	bad := []string{
		"name < 1",        // 字符串与数字不可比
		"name + 1 == 'a'", // 字符串与数字不可加
		"10 % 0 == 0",     // 取模零
		"'x' in 'xyz'",    // 右侧非数组
		"!days",           // 未定义变量不是 bool
		"-name < 0",       // 一元负号要求数字
	}
	for _, expr := range bad {
		if _, err := ev.EvalBool(ctx, expr, env); err == nil {
			t.Errorf("EvalBool(%q) should fail", expr)
		}
	}
}

// TestLookupPath 点路径解析：嵌套 map、Vars 值、缺失、标量穿透失败。
func TestLookupPath(t *testing.T) {
	env := Vars{
		"a": map[string]any{"b": map[string]any{"c": float64(7)}},
		"v": Vars{"x": "val"},
		"n": float64(1),
	}
	cases := []struct {
		path []string
		want any
	}{
		{[]string{"a", "b", "c"}, float64(7)},
		{[]string{"v", "x"}, "val"},
		{[]string{"n"}, float64(1)},
		{[]string{"a", "zzz"}, nil},
		{[]string{"n", "deeper"}, nil}, // 标量不能继续下钻
		{[]string{"missing"}, nil},
	}
	for _, tc := range cases {
		if got := lookupPath(env, tc.path); got != tc.want {
			t.Errorf("lookupPath(%v) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// TestValueEqual 跨类型相等语义：nil、bool、string、数值互转。
func TestValueEqual(t *testing.T) {
	cases := []struct {
		l, r any
		want bool
	}{
		{nil, nil, true},
		{nil, 0, false},
		{true, true, true},
		{true, false, false},
		{true, "true", false},
		{"a", "a", true},
		{"1", 1, false}, // 字符串不转数值
		{float64(2), 2, true},
		{int64(3), uint8(3), true},
		{float64(1), "1", false},
		{[]any{1}, []any{1}, false}, // 复合类型不支持
	}
	for _, tc := range cases {
		if got := valueEqual(tc.l, tc.r); got != tc.want {
			t.Errorf("valueEqual(%#v, %#v) = %v, want %v", tc.l, tc.r, got, tc.want)
		}
	}
}

// TestSimpleEvaluatorParseErrors 语法错误全部 fail-closed。
func TestSimpleEvaluatorParseErrors(t *testing.T) {
	ev := NewSimpleEvaluator()
	ctx := context.Background()
	bad := []string{
		"(((1)",         // 括号不闭合
		"'unterminated", // 字符串未闭合
		"1 +",           // 缺右操作数
		"&& true",       // 缺左操作数
		"a in",          // 缺右侧
		"a ?",           // 非法字符
		"1 2",           // 多余 token
		"[1, 2",         // 数组未闭合
	}
	for _, expr := range bad {
		if _, err := ev.EvalBool(ctx, expr, Vars{}); err == nil {
			t.Errorf("EvalBool(%q) should fail", expr)
		} else if !strings.Contains(err.Error(), "flow:") {
			t.Errorf("EvalBool(%q) error should be wrapped: %v", expr, err)
		}
	}
}
