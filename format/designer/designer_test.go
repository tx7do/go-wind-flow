package designer

import (
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

const sampleDoc = `{
  "version": 1,
  "key": "leave",
  "name": "请假审批",
  "nodes": [
    {"id": "start", "type": "startEvent", "name": "提交", "ui": {"x": 100, "y": 200}},
    {"id": "leader", "type": "userTask", "name": "直属领导审批",
     "approver": {"rules": [{"kind": "LEADER"}], "strategy": "ALL"},
     "ui": {"x": 300, "y": 200}},
    {"id": "gw", "type": "exclusiveGateway", "name": "是否超过3天", "ui": {"x": 500, "y": 200}},
    {"id": "end", "type": "endEvent", "ui": {"x": 900, "y": 200}}
  ],
  "edges": [
    {"id": "f1", "source": "start", "target": "leader", "ui": {"waypoints": [[200, 200]]}},
    {"id": "f2", "source": "leader", "target": "gw"},
    {"id": "f3", "source": "gw", "target": "end", "isDefault": true},
    {"id": "f4", "source": "gw", "target": "end", "condition": "days > 3"}
  ]
}`

// 条件边须指向不同目标才可通过部署校验——测试里补一个总监节点。
const sampleDocFull = `{
  "version": 1,
  "key": "leave",
  "name": "请假审批",
  "nodes": [
    {"id": "start", "type": "startEvent", "ui": {"x": 100, "y": 200}},
    {"id": "leader", "type": "userTask", "name": "直属领导审批",
     "approver": {"rules": [{"kind": "LEADER"}], "strategy": "ALL"}, "ui": {"x": 300, "y": 200}},
    {"id": "gw", "type": "exclusiveGateway", "defaultEdge": "f3", "ui": {"x": 500, "y": 200}},
    {"id": "director", "type": "userTask", "name": "总监审批",
     "approver": {"rules": [{"kind": "USER", "value": "u_1001"}], "strategy": "ANY"}, "ui": {"x": 700, "y": 100}},
    {"id": "end", "type": "endEvent", "ui": {"x": 900, "y": 200}}
  ],
  "edges": [
    {"id": "f1", "source": "start", "target": "leader"},
    {"id": "f2", "source": "leader", "target": "gw"},
    {"id": "f3", "source": "gw", "target": "end", "isDefault": true},
    {"id": "f4", "source": "gw", "target": "director", "condition": "days > 3"},
    {"id": "f5", "source": "director", "target": "end"}
  ]
}`

func TestParseAndValidate(t *testing.T) {
	def, err := Parse([]byte(sampleDocFull))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if def.Key != "leave" || len(def.Nodes) != 5 || len(def.Edges) != 5 {
		t.Fatalf("parse result wrong: key=%s nodes=%d edges=%d", def.Key, len(def.Nodes), len(def.Edges))
	}
	leader := def.Nodes["leader"]
	if leader.Approver == nil || leader.Approver.Rules[0].Kind != flow.ApproverLeader {
		t.Fatalf("approver rules lost: %+v", leader.Approver)
	}
	gw := def.Nodes["gw"]
	if gw.DefaultEdge != "f3" {
		t.Fatalf("defaultEdge lost: %q", gw.DefaultEdge)
	}
	if err := flow.ValidateDefinition(def); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Parse([]byte(`{"version": 2, "key": "x", "nodes": [], "edges": []}`)); err == nil {
		t.Fatal("wrong version should fail")
	}
	if _, err := Parse([]byte(`{"version": 1, "key": "", "nodes": [], "edges": []}`)); err == nil {
		t.Fatal("missing key should fail")
	}
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Fatal("bad json should fail")
	}
}

func TestRoundTrip(t *testing.T) {
	def, err := Parse([]byte(sampleDocFull))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Write(def)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	re, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, out)
	}
	if len(re.Nodes) != len(def.Nodes) || len(re.Edges) != len(def.Edges) {
		t.Fatalf("round-trip counts mismatch")
	}
	if re.Nodes["director"].Approver.Rules[0].Value != "u_1001" {
		t.Fatal("round-trip lost approver value")
	}
	// 稳定输出：两次 Write 逐字节一致。
	out2, _ := Write(def)
	if string(out) != string(out2) {
		t.Fatal("Write is not deterministic")
	}
}
