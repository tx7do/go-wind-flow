package bpmn

import (
	"os"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

func TestParseExampleBPMN(t *testing.T) {
	data, err := os.ReadFile("../../examples/leave-approval.bpmn.xml")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	def, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if def.Name != "请假审批" {
		t.Errorf("name = %q", def.Name)
	}
	if len(def.Nodes) != 5 {
		t.Errorf("nodes = %d, want 5", len(def.Nodes))
	}
	if len(def.Edges) != 5 {
		t.Errorf("edges = %d, want 5", len(def.Edges))
	}

	leader := def.Nodes["leader_review"]
	if leader == nil || leader.Type != flow.NodeUserTask {
		t.Fatalf("leader_review missing")
	}
	if leader.Approver == nil || len(leader.Approver.Rules) != 1 ||
		leader.Approver.Rules[0].Kind != flow.ApproverLeader {
		t.Errorf("leader_review approver rules wrong: %+v", leader.Approver)
	}
	if leader.Approval.Strategy != flow.StrategyAll {
		t.Errorf("strategy = %s, want ALL", leader.Approval.Strategy)
	}

	gw := def.Nodes["days_gateway"]
	if gw == nil || gw.Type != flow.NodeExclusiveGateway {
		t.Fatalf("days_gateway missing")
	}
	if gw.DefaultEdge != "gw_default" {
		t.Errorf("default edge = %q", gw.DefaultEdge)
	}
	var longEdge *flow.Edge
	for _, e := range def.Edges {
		if e.ID == "f_long" {
			longEdge = e
		}
		if e.ID == "gw_default" && !e.IsDefault {
			t.Errorf("gw_default should be default")
		}
	}
	if longEdge == nil || longEdge.Condition != "days > 3" {
		t.Errorf("f_long condition = %+v", longEdge)
	}

	if err := flow.ValidateDefinition(def); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	data, err := os.ReadFile("../../examples/leave-approval.bpmn.xml")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	def, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Write(def)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	re, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse: %v\n--- XML ---\n%s", err, string(out))
	}
	if len(re.Nodes) != len(def.Nodes) || len(re.Edges) != len(def.Edges) {
		t.Fatalf("round-trip node/edge count mismatch: %d/%d vs %d/%d",
			len(re.Nodes), len(re.Edges), len(def.Nodes), len(def.Edges))
	}
	if re.Nodes["leader_review"].Approver.Rules[0].Kind != flow.ApproverLeader {
		t.Errorf("round-trip lost approver rules")
	}
}
