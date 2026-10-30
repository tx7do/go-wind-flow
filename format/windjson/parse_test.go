package windjson

import (
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

func TestParseGraphV2(t *testing.T) {
	data := []byte(`{
		"version": 2,
		"nodes": [
			{"id": "start", "type": "START"},
			{"id": "tl", "type": "TASK", "approvers": [{"type": "LEADER"}], "strategy": "ALL"},
			{"id": "gw", "type": "EXCLUSIVE_GATEWAY"},
			{"id": "hr", "type": "TASK", "approvers": [{"type": "POSITION", "code": "HRD"}], "strategy": "ANY"},
			{"id": "sp", "type": "SUBPROCESS", "subprocessDefinition": "child_flow:3"},
			{"id": "end", "type": "END"}
		],
		"edges": [
			{"from": "start", "to": "tl"},
			{"from": "tl", "to": "gw"},
			{"from": "gw", "to": "hr", "condition": "amount > 1000"},
			{"from": "gw", "to": "end", "condition": "default"},
			{"from": "hr", "to": "sp"},
			{"from": "sp", "to": "end"}
		]
	}`)
	def, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(def.Nodes) != 6 || len(def.Edges) != 6 {
		t.Fatalf("nodes=%d edges=%d, want 6/6", len(def.Nodes), len(def.Edges))
	}
	tl := def.Nodes["tl"]
	if tl.Type != flow.NodeUserTask || tl.Approver.Rules[0].Kind != flow.ApproverLeader {
		t.Errorf("tl node wrong: %+v", tl)
	}
	hr := def.Nodes["hr"]
	if hr.Approver.Rules[0].Kind != flow.ApproverPosition || hr.Approver.Rules[0].Value != "HRD" {
		t.Errorf("hr approver wrong: %+v", hr.Approver.Rules)
	}
	if hr.Approval.Strategy != flow.StrategyAny {
		t.Errorf("hr strategy = %s, want ANY", hr.Approval.Strategy)
	}
	sp := def.Nodes["sp"]
	if sp.Type != flow.NodeCallActivity || sp.SubprocessKey != "child_flow" || sp.SubprocessVersion != 3 {
		t.Errorf("subprocess ref wrong: %+v", sp)
	}
	for _, e := range def.Edges {
		if e.From == "gw" && e.To == "end" && !e.IsDefault {
			t.Errorf("default edge not marked")
		}
		if e.From == "gw" && e.To == "hr" && e.Condition != "amount > 1000" {
			t.Errorf("condition lost: %q", e.Condition)
		}
	}
	if err := flow.ValidateDefinition(def); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestParseLegacyArray(t *testing.T) {
	data := []byte(`[
		{"approver_type": "USER", "approver": 42, "strategy": "ALL"},
		{"approvers": [{"type": "ROLE", "code": "DBA"}], "strategy": "ANY"}
	]`)
	def, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(def.Nodes) != 4 || len(def.Edges) != 3 {
		t.Fatalf("nodes=%d edges=%d, want 4/3", len(def.Nodes), len(def.Edges))
	}
	n0 := def.Nodes["__legacy_node_0"]
	if n0.Approver.Rules[0].Kind != flow.ApproverUser || n0.Approver.Rules[0].Value != "42" {
		t.Errorf("legacy USER rule wrong: %+v", n0.Approver.Rules)
	}
	n1 := def.Nodes["__legacy_node_1"]
	if n1.Approver.Rules[0].Kind != flow.ApproverRole {
		t.Errorf("legacy ROLE rule wrong: %+v", n1.Approver.Rules)
	}
	if err := flow.ValidateDefinition(def); err != nil {
		t.Errorf("validate: %v", err)
	}
}
