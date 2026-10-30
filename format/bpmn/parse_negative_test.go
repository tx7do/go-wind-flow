package bpmn

import (
	"strings"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

// wrapShell 以最小合法外壳包裹流程体。
func wrapShell(inner string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p1" isExecutable="true">` + inner + `</process>
</definitions>`)
}

// 每条负向用例：输入流程体片段 + 期望错误子串。
func TestParseNegative(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{"非法 XML", []byte("not xml at all"), "parse bpmn xml"},
		{"无 process", []byte(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"/>`), "no <process> in bpmn xml"},
		{"中间定时器缺时长", wrapShell(`<startEvent id="s"/><intermediateCatchEvent id="w"><timerEventDefinition/></intermediateCatchEvent><endEvent id="e"/>`), "intermediateCatchEvent w missing timeDuration"},
		{"中间定时器时长非法", wrapShell(`<startEvent id="s"/><intermediateCatchEvent id="w"><timerEventDefinition><timeDuration>xyz</timeDuration></timerEventDefinition></intermediateCatchEvent><endEvent id="e"/>`), "intermediateCatchEvent w"},
		{"中间消息缺 messageRef", wrapShell(`<startEvent id="s"/><intermediateCatchEvent id="w"><messageEventDefinition/></intermediateCatchEvent><endEvent id="e"/>`), "intermediateCatchEvent w missing messageRef"},
		{"中间事件不支持的触发器", wrapShell(`<startEvent id="s"/><intermediateCatchEvent id="w"><signalEventDefinition/></intermediateCatchEvent><endEvent id="e"/>`), "intermediateCatchEvent w has no supported eventDefinition"},
		{"边界事件缺触发器", wrapShell(`<startEvent id="s"/><userTask id="t"/><boundaryEvent id="b" attachedToRef="t"/><endEvent id="e"/>`), "boundaryEvent b missing timerEventDefinition/timeDuration"},
		{"边界事件时长非法", wrapShell(`<startEvent id="s"/><userTask id="t"/><boundaryEvent id="b" attachedToRef="t"><timerEventDefinition><timeDuration>bad</timeDuration></timerEventDefinition></boundaryEvent><endEvent id="e"/>`), "boundaryEvent b"},
		{"子流程缺 id", wrapShell(`<startEvent id="s"/><endEvent id="e"/><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><subProcess><startEvent id="sps"/><endEvent id="spe"/></subProcess>`), "embedded subProcess missing id"},
		{"子流程双 start", wrapShell(`<startEvent id="s"/><endEvent id="e"/><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><subProcess id="sp"><startEvent id="a"/><startEvent id="b"/><endEvent id="spe"/></subProcess>`), "embedded subProcess sp must have exactly 1 startEvent"},
		{"子流程 error end", wrapShell(`<startEvent id="s"/><endEvent id="e"/><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><subProcess id="sp"><startEvent id="sps"/><endEvent id="spe"><errorEventDefinition errorRef="E1"/></endEvent></subProcess>`), "embedded subProcess sp error end unsupported"},
		{"子流程缺 end", wrapShell(`<startEvent id="s"/><endEvent id="e"/><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><subProcess id="sp"><startEvent id="sps"/></subProcess>`), "embedded subProcess sp must have exactly 1 endEvent"},
		{"边界挂子流程", wrapShell(`<startEvent id="s"/><boundaryEvent id="b" attachedToRef="sp"><timerEventDefinition><timeDuration>PT1H</timeDuration></timerEventDefinition></boundaryEvent><endEvent id="e"/><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><subProcess id="sp"><startEvent id="sps"/><userTask id="t"/><endEvent id="spe"/><sequenceFlow id="g1" sourceRef="sps" targetRef="t"/><sequenceFlow id="g2" sourceRef="t" targetRef="spe"/></subProcess>`), "boundary b attached to embedded subProcess sp unsupported"},
		{"子流程缺内部流", wrapShell(`<startEvent id="s"/><endEvent id="e"/><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><subProcess id="sp"><startEvent id="sps"/><userTask id="t"/><endEvent id="spe"/></subProcess>`), "embedded subProcess sp missing inner entry/exit flow"},
		{"扁平化后节点 ID 冲突", wrapShell(`<startEvent id="s"/><userTask id="sp.t1"/><endEvent id="e"/><sequenceFlow id="f1" sourceRef="s" targetRef="sp.t1"/><sequenceFlow id="f2" sourceRef="sp.t1" targetRef="e"/><subProcess id="sp"><startEvent id="sps"/><userTask id="t1"/><endEvent id="spe"/><sequenceFlow id="g1" sourceRef="sps" targetRef="t1"/><sequenceFlow id="g2" sourceRef="t1" targetRef="spe"/></subProcess>`), "duplicate node id sp.t1 after flatten"},
	}
	for _, tc := range cases {
		_, err := Parse(tc.data)
		if err == nil {
			t.Errorf("%s: expected error", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error %q missing %q", tc.name, err.Error(), tc.wantErr)
		}
	}
}

// TestParseFlattenSubProcess SES 合法子流程扁平化：入口/出口重定向。
func TestParseFlattenSubProcess(t *testing.T) {
	data := wrapShell(`
		<startEvent id="ps"/>
		<userTask id="outer"/>
		<endEvent id="pe"/>
		<sequenceFlow id="f1" sourceRef="ps" targetRef="sp"/>
		<sequenceFlow id="f2" sourceRef="sp" targetRef="outer"/>
		<sequenceFlow id="f3" sourceRef="outer" targetRef="pe"/>
		<subProcess id="sp">
			<startEvent id="is"/>
			<userTask id="t1"/>
			<userTask id="t2"/>
			<endEvent id="ie"/>
			<sequenceFlow id="g1" sourceRef="is" targetRef="t1"/>
			<sequenceFlow id="g2" sourceRef="t1" targetRef="t2"/>
			<sequenceFlow id="g3" sourceRef="t2" targetRef="ie"/>
		</subProcess>`)
	def, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := def.Nodes["sp"]; ok {
		t.Error("subProcess itself should not become a node")
	}
	for _, id := range []string{"sp.t1", "sp.t2"} {
		if def.Nodes[id] == nil {
			t.Errorf("node %s missing after flatten", id)
		}
	}
	edge := func(id string) *flow.Edge { return def.Edge(id) }
	if e := edge("f1"); e == nil || e.To != "sp.t1" || e.From != "ps" {
		t.Errorf("entry redirect wrong: %+v", e)
	}
	if e := edge("f2"); e == nil || e.From != "sp.t2" || e.To != "outer" {
		t.Errorf("exit redirect wrong: %+v", e)
	}
	if e := edge("sp.g2"); e == nil || e.From != "sp.t1" || e.To != "sp.t2" {
		t.Errorf("inner edge prefix wrong: %+v", e)
	}
}

// TestParseAttributeFallbacks Activiti 风格属性与缺省流、缺省 ID 等解析兜底。
func TestParseAttributeFallbacks(t *testing.T) {
	data := wrapShell(`
		<startEvent id="s"/>
		<userTask id="t1" name="审批" assignee="alice" candidateGroups="finance, boss"/>
		<exclusiveGateway id="g" default="f3"/>
		<endEvent id="e"/>
		<sequenceFlow id="f1" sourceRef="s" targetRef="t1"/>
		<sequenceFlow id="f2" sourceRef="t1" targetRef="g"/>
		<sequenceFlow id="f3" sourceRef="g" targetRef="e"/>
		<sequenceFlow sourceRef="g" targetRef="t1"/>`)
	def, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	t1 := def.Nodes["t1"]
	if t1 == nil {
		t.Fatal("t1 missing")
	}
	kinds := map[flow.ApproverKind]int{}
	for _, r := range t1.Approver.Rules {
		kinds[r.Kind]++
	}
	if kinds[flow.ApproverUser] != 1 || kinds[flow.ApproverGroup] != 2 {
		t.Errorf("approver rules wrong: %+v", t1.Approver.Rules)
	}

	// 缺省流：default="f3" 的出边标 IsDefault。
	if e := def.Edge("f3"); e == nil || !e.IsDefault {
		t.Errorf("f3 should be default: %+v", e)
	}
	// 无 ID 顺序流自动编号。
	gen := def.Edge("flow_4")
	if gen == nil || gen.From != "g" || gen.To != "t1" {
		t.Errorf("generated flow id wrong: %+v", gen)
	}
}

// TestParseFeatureElements 定时启动、错误终点、脚本作用域、子流程版本引用。
func TestParseFeatureElements(t *testing.T) {
	data := wrapShell(`
		<startEvent id="s"><timerEventDefinition><timeDuration>PT30M</timeDuration></timerEventDefinition></startEvent>
		<userTask id="t"/>
		<scriptTask id="sc" scriptFormat="js" flow:scope="local" xmlns:flow="https://github.com/tx7do/go-wind-flow"><script>return {};</script></scriptTask>
		<callActivity id="ca" calledElement="bonus:v2"/>
		<boundaryEvent id="b" attachedToRef="t" cancelActivity="false"><timerEventDefinition><timeDuration>PT1H</timeDuration></timerEventDefinition></boundaryEvent>
		<endEvent id="ee"><errorEventDefinition errorRef="E1"/></endEvent>
		<endEvent id="ok"/>
		<sequenceFlow id="f1" sourceRef="s" targetRef="t"/>
		<sequenceFlow id="f2" sourceRef="t" targetRef="sc"/>
		<sequenceFlow id="f3" sourceRef="sc" targetRef="ca"/>
		<sequenceFlow id="f4" sourceRef="ca" targetRef="ee"/>
		<sequenceFlow id="f5" sourceRef="b" targetRef="ok"/>`)
	def, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s := def.Nodes["s"]; s == nil || s.Type != flow.NodeStartEvent || s.Timer == nil || s.Timer.Duration.Std() != 30*60*1e9 {
		t.Errorf("timer start wrong: %+v", s)
	}
	sc := def.Nodes["sc"]
	if sc == nil || sc.Script == nil || sc.Script.Scope != "local" || sc.Script.Lang != "js" || sc.Script.Code != "return {};" {
		t.Errorf("script task wrong: %+v", sc)
	}
	ca := def.Nodes["ca"]
	if ca == nil || ca.SubprocessKey != "bonus" || ca.SubprocessVersion != 2 {
		t.Errorf("callActivity version ref wrong: %+v", ca)
	}
	b := def.Nodes["b"]
	if b == nil || b.Interrupting == nil || *b.Interrupting {
		t.Errorf("boundary cancelActivity=false not respected: %+v", b)
	}
	if ee := def.Nodes["ee"]; ee == nil || ee.Type != flow.NodeErrorEndEvent || ee.ErrorCode != "E1" {
		t.Errorf("error end wrong: %+v", ee)
	}
}

func TestStripUEL(t *testing.T) {
	cases := map[string]string{
		"${amount > 100}": "amount > 100",
		"  x  ":           "x",
		"a == b":          "a == b",
		"${unclosed":      "${unclosed",
	}
	for in, want := range cases {
		if got := stripUEL(in); got != want {
			t.Errorf("stripUEL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitCalledElement(t *testing.T) {
	cases := []struct {
		in      string
		key     string
		version int
	}{
		{"key", "key", 0},
		{"key:3", "key", 3},
		{"key:v3", "key", 3},
		{"ns:key:2", "ns:key", 2},
		{"key:abc", "key:abc", 0},
	}
	for _, tc := range cases {
		k, v := splitCalledElement(tc.in)
		if k != tc.key || v != tc.version {
			t.Errorf("splitCalledElement(%q) = (%q, %d), want (%q, %d)", tc.in, k, v, tc.key, tc.version)
		}
	}
}

// TestImporterExporterWrappers SPI 包装器转发正确。
func TestImporterExporterWrappers(t *testing.T) {
	var imp Importer
	if imp.Format() != FormatName {
		t.Errorf("importer format = %q", imp.Format())
	}
	data := wrapShell(`<startEvent id="s"/><endEvent id="e"/><sequenceFlow id="f1" sourceRef="s" targetRef="e"/>`)
	def, err := imp.Import(data)
	if err != nil || def == nil || len(def.Nodes) != 2 {
		t.Fatalf("importer import: %v", err)
	}

	var exp Exporter
	if exp.Format() != FormatName {
		t.Errorf("exporter format = %q", exp.Format())
	}
	out, err := exp.Export(def)
	if err != nil || len(out) == 0 {
		t.Fatalf("exporter export: %v", err)
	}
}
