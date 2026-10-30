package flow

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// base 构造最小定义骨架。
func baseDef(nodes map[string]*Node, edges []*Edge) *Definition {
	return &Definition{Key: "k", Nodes: nodes, Edges: edges}
}

func user(id string) *Node { return &Node{ID: id, Type: NodeUserTask} }

// TestValidateErrors 覆盖校验器的每一条错误分支（与 validate.go 的规则顺序对齐）。
func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name    string
		def     *Definition
		wantErr string
	}{
		{"空定义", &Definition{Key: "k"}, "no nodes"},
		{"节点缺类型", baseDef(
			map[string]*Node{"x": {ID: "x"}},
			nil,
		), "node missing id or type"},
		{"未知节点类型", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"x": {ID: "x", Type: NodeType("nope")},
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "x"}, {ID: "f2", From: "x", To: "e"}},
		), `unknown node type "nope"`},
		{"双开始", baseDef(
			map[string]*Node{
				"s1": {ID: "s1", Type: NodeStartEvent},
				"s2": {ID: "s2", Type: NodeStartEvent},
				"e":  {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s1", To: "e"}, {ID: "f2", From: "s2", To: "e"}},
		), "want exactly 1 startEvent, got 2"},
		{"零终点", baseDef(
			map[string]*Node{"s": {ID: "s", Type: NodeStartEvent}},
			nil,
		), "want at least 1 endEvent, got 0"},
		{"边的端点不存在", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "ghost"}},
		), "edge f1 references unknown node"},
		{"开始节点多出边", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"e1": {ID: "e1", Type: NodeEndEvent},
				"e2": {ID: "e2", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "e1"}, {ID: "f2", From: "s", To: "e2"}},
		), "startEvent s must have 0 in / 1 out"},
		{"结束节点带出边", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"e1": {ID: "e1", Type: NodeEndEvent},
				"g":  {ID: "g", Type: NodeParallelGateway},
				"u1": user("u1"),
				"u2": user("u2"),
				"e2": {ID: "e2", Type: NodeEndEvent},
				"e3": {ID: "e3", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "e1"}, {ID: "f2", From: "e1", To: "g"},
				{ID: "f3", From: "g", To: "u1"}, {ID: "f4", From: "g", To: "u2"},
				{ID: "f5", From: "u1", To: "e2"}, {ID: "f6", From: "u2", To: "e3"},
			},
		), "endEvent e1 must have ≥1 in / 0 out"},
		{"任务多出边", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"t":  user("t"),
				"e1": {ID: "e1", Type: NodeEndEvent},
				"e2": {ID: "e2", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "t"},
				{ID: "f2", From: "t", To: "e1"}, {ID: "f3", From: "t", To: "e2"},
			},
		), "node t (userTask) must have 1 in / 1 out"},
		{"脚本任务缺脚本", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"t": {ID: "t", Type: NodeScriptTask},
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "t"}, {ID: "f2", From: "t", To: "e"}},
		), "script task t missing script spec"},
		{"callActivity 缺子流程", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"c": {ID: "c", Type: NodeCallActivity},
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "c"}, {ID: "f2", From: "c", To: "e"}},
		), "callActivity c missing subprocess key"},
		{"消息事件缺消息名", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"m": {ID: "m", Type: NodeMessageEvent},
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "m"}, {ID: "f2", From: "m", To: "e"}},
		), "message event m missing message name"},
		{"定时器缺时长", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"m": {ID: "m", Type: NodeTimerEvent},
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "m"}, {ID: "f2", From: "m", To: "e"}},
		), "timer event m missing positive duration"},
		{"errorEnd 缺错误码", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"pf": {ID: "pf", Type: NodeParallelGateway},
				"t":  user("t"),
				"x":  {ID: "x", Type: NodeErrorEndEvent},
				"e":  {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "pf"},
				{ID: "f2", From: "pf", To: "t"}, {ID: "f3", From: "pf", To: "e"},
				{ID: "f4", From: "t", To: "x"},
			},
		), "errorEndEvent x missing error code"},
		{"边界事件缺挂接目标", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"u":  user("u"),
				"e":  {ID: "e", Type: NodeEndEvent},
				"b":  {ID: "b", Type: NodeBoundaryEvent, Timer: &TimerSpec{Duration: Duration(time.Hour)}},
				"e2": {ID: "e2", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "u"}, {ID: "f2", From: "u", To: "e"},
				{ID: "f3", From: "b", To: "e2"},
			},
		), "boundary event b missing attachedToRef"},
		{"边界事件挂接目标不存在", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"u":  user("u"),
				"e":  {ID: "e", Type: NodeEndEvent},
				"b":  {ID: "b", Type: NodeBoundaryEvent, AttachedTo: "ghost", Timer: &TimerSpec{Duration: Duration(time.Hour)}},
				"e2": {ID: "e2", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "u"}, {ID: "f2", From: "u", To: "e"},
				{ID: "f3", From: "b", To: "e2"},
			},
		), "boundary event b attached to unknown node ghost"},
		{"错误边界挂 userTask", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"u":  user("u"),
				"e":  {ID: "e", Type: NodeEndEvent},
				"b":  {ID: "b", Type: NodeBoundaryEvent, AttachedTo: "u", ErrorCode: "E1"},
				"e2": {ID: "e2", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "u"}, {ID: "f2", From: "u", To: "e"},
				{ID: "f3", From: "b", To: "e2"},
			},
		), "error boundary b can only attach to callActivity, got userTask"},
		{"定时边界挂 callActivity", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"c":  {ID: "c", Type: NodeCallActivity, SubprocessKey: "sub"},
				"e":  {ID: "e", Type: NodeEndEvent},
				"b":  {ID: "b", Type: NodeBoundaryEvent, AttachedTo: "c", Timer: &TimerSpec{Duration: Duration(time.Hour)}},
				"e2": {ID: "e2", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "c"}, {ID: "f2", From: "c", To: "e"},
				{ID: "f3", From: "b", To: "e2"},
			},
		), "timer boundary b can only attach to userTask, got callActivity"},
		{"边界事件缺触发器", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"u":  user("u"),
				"e":  {ID: "e", Type: NodeEndEvent},
				"b":  {ID: "b", Type: NodeBoundaryEvent, AttachedTo: "u"},
				"e2": {ID: "e2", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "u"}, {ID: "f2", From: "u", To: "e"},
				{ID: "f3", From: "b", To: "e2"},
			},
		), "boundary event b missing trigger"},
		{"退化排他网关", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"g": {ID: "g", Type: NodeExclusiveGateway},
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "g"}, {ID: "f2", From: "g", To: "e"}},
		), "exclusiveGateway g must have ≥2 out edges"},
		{"排他网关缺默认流", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"g": {ID: "g", Type: NodeExclusiveGateway},
				"a": user("a"),
				"b": user("b"),
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "g"},
				{ID: "f2", From: "g", To: "a", Condition: "${x > 1}"},
				{ID: "f3", From: "g", To: "b", Condition: "${x <= 1}"},
				{ID: "f4", From: "a", To: "e"}, {ID: "f5", From: "b", To: "e"},
			},
		), "exclusiveGateway g must have exactly 1 default out edge"},
		{"排他网关出边缺条件", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"g": {ID: "g", Type: NodeExclusiveGateway},
				"a": user("a"),
				"b": user("b"),
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "g"},
				{ID: "f2", From: "g", To: "a"},
				{ID: "f3", From: "g", To: "b", IsDefault: true},
				{ID: "f4", From: "a", To: "e"}, {ID: "f5", From: "b", To: "e"},
			},
		), "exclusiveGateway g out edge f2 missing condition"},
		{"并行分叉带条件", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"pf": {ID: "pf", Type: NodeParallelGateway},
				"u1": user("u1"),
				"u2": user("u2"),
				"e":  {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "pf"},
				{ID: "f2", From: "pf", To: "u1", Condition: "${x}"},
				{ID: "f3", From: "pf", To: "u2"},
				{ID: "f4", From: "u1", To: "e"}, {ID: "f5", From: "u2", To: "e"},
			},
		), "parallel fork pf out edge f2 must not carry condition"},
		{"并行网关退化", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"g": {ID: "g", Type: NodeParallelGateway},
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{{ID: "f1", From: "s", To: "g"}, {ID: "f2", From: "g", To: "e"}},
		), "gateway g degenerated (in=1 out=1)"},
		{"汇合网关多出边", baseDef(
			map[string]*Node{
				"s": {ID: "s", Type: NodeStartEvent},
				"g": {ID: "g", Type: NodeParallelGateway},
				"a": user("a"),
				"b": user("b"),
				"e": {ID: "e", Type: NodeEndEvent},
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "g"},
				{ID: "f2", From: "g", To: "a"}, {ID: "f3", From: "g", To: "b"},
				{ID: "f4", From: "a", To: "g"}, {ID: "f5", From: "b", To: "g"},
				{ID: "f6", From: "g", To: "e"},
			},
		), "gateway g as join must have exactly 1 out edge"},
		{"孤岛节点", baseDef(
			map[string]*Node{
				"s":  {ID: "s", Type: NodeStartEvent},
				"e":  {ID: "e", Type: NodeEndEvent},
				"u1": user("u1"),
				"u2": user("u2"),
			},
			[]*Edge{
				{ID: "f1", From: "s", To: "e"},
				{ID: "f2", From: "u1", To: "u2"}, {ID: "f3", From: "u2", To: "u1"},
			},
		), "not reachable from start"},
	}
	for _, tc := range cases {
		err := ValidateDefinition(tc.def)
		if err == nil {
			t.Errorf("%s: expected error", tc.name)
			continue
		}
		if !errors.Is(err, ErrInvalidDefinition) {
			t.Errorf("%s: error %q not wrapped by ErrInvalidDefinition", tc.name, err.Error())
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error %q missing %q", tc.name, err.Error(), tc.wantErr)
		}
	}
}

// TestValidateGatewayModes 汇合排他网关（多入单出）合法。
func TestValidateGatewayModes(t *testing.T) {
	merge := baseDef(
		map[string]*Node{
			"s":  {ID: "s", Type: NodeStartEvent},
			"g1": {ID: "g1", Type: NodeExclusiveGateway},
			"a":  user("a"),
			"b":  user("b"),
			"gm": {ID: "gm", Type: NodeExclusiveGateway},
			"e":  {ID: "e", Type: NodeEndEvent},
		},
		[]*Edge{
			{ID: "f1", From: "s", To: "g1"},
			{ID: "f2", From: "g1", To: "a", Condition: "${amount > 100}"},
			{ID: "f3", From: "g1", To: "b", IsDefault: true},
			{ID: "f4", From: "a", To: "gm"}, {ID: "f5", From: "b", To: "gm"},
			{ID: "f6", From: "gm", To: "e"},
		},
	)
	if err := ValidateDefinition(merge); err != nil {
		t.Fatalf("merge gateway should be valid: %v", err)
	}
}

// TestValidateFullFeatured 全特性合法定义：定时/错误边界、包容网关、错误终点。
func TestValidateFullFeatured(t *testing.T) {
	def := baseDef(
		map[string]*Node{
			"s":  {ID: "s", Type: NodeStartEvent},
			"t":  {ID: "t", Type: NodeUserTask},
			"b":  {ID: "b", Type: NodeBoundaryEvent, AttachedTo: "t", Interrupting: &falsch, Timer: &TimerSpec{Duration: Duration(24 * time.Hour)}},
			"up": user("up"),
			"g":  {ID: "g", Type: NodeInclusiveGateway},
			"a1": user("a1"),
			"a2": user("a2"),
			"c":  {ID: "c", Type: NodeCallActivity, SubprocessKey: "sub"},
			"eb": {ID: "eb", Type: NodeBoundaryEvent, AttachedTo: "c", ErrorCode: "E1"},
			"h":  user("h"),
			"ee": {ID: "ee", Type: NodeErrorEndEvent, ErrorCode: "E1"},
			"e":  {ID: "e", Type: NodeEndEvent},
		},
		[]*Edge{
			{ID: "f1", From: "s", To: "t"},
			{ID: "f2", From: "t", To: "g"},
			{ID: "f3", From: "b", To: "up"},
			{ID: "f4", From: "g", To: "a1"}, {ID: "f5", From: "g", To: "a2"},
			{ID: "f6", From: "a1", To: "c"},
			{ID: "f7", From: "a2", To: "ee"},
			{ID: "f8", From: "c", To: "ee"},
			{ID: "f9", From: "eb", To: "h"},
			{ID: "f10", From: "up", To: "e"}, {ID: "f11", From: "h", To: "e"},
		},
	)
	if err := ValidateDefinition(def); err != nil {
		t.Fatalf("full-featured definition should be valid: %v", err)
	}
}

// TestValidateLoopWithExit 带出口的重试环：回边经汇合网关承接，可部署。
func TestValidateLoopWithExit(t *testing.T) {
	def := baseDef(
		map[string]*Node{
			"s":  {ID: "s", Type: NodeStartEvent},
			"pf": {ID: "pf", Type: NodeParallelGateway},
			"a":  user("a"),
			"b":  user("b"),
			"mj": {ID: "mj", Type: NodeExclusiveGateway},
			"t1": user("t1"),
			"t2": user("t2"),
			"g2": {ID: "g2", Type: NodeExclusiveGateway},
			"e":  {ID: "e", Type: NodeEndEvent},
			"e2": {ID: "e2", Type: NodeEndEvent},
		},
		[]*Edge{
			{ID: "f1", From: "s", To: "pf"},
			{ID: "f2", From: "pf", To: "a"}, {ID: "f3", From: "pf", To: "b"},
			{ID: "f4", From: "a", To: "mj"},
			{ID: "f5", From: "mj", To: "t1"},
			{ID: "f6", From: "t1", To: "t2"},
			{ID: "f7", From: "t2", To: "g2"},
			{ID: "f8", From: "g2", To: "mj", Condition: "${retry}"},
			{ID: "f9", From: "g2", To: "e", IsDefault: true},
			{ID: "f10", From: "b", To: "e2"},
		},
	)
	if err := ValidateDefinition(def); err != nil {
		t.Fatalf("loop with exit should be valid: %v", err)
	}
}

var falsch = false
