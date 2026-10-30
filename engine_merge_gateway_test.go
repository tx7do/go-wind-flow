package go_wind_flow_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

// 汇合排他网关（多入单出）：审批树分支汇合的底座。
var mergeGatewayXML = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <process id="merge_gw" isExecutable="true">
    <startEvent id="s"/>
    <exclusiveGateway id="split" default="b_def"/>
    <userTask id="a"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"ua"}]}</flow:assignees></extensionElements></userTask>
    <userTask id="b"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"ub"}]}</flow:assignees></extensionElements></userTask>
    <exclusiveGateway id="merge" default="mout"/>
    <userTask id="after"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"uz"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="split"/>
    <sequenceFlow id="b_hi" sourceRef="split" targetRef="a">
      <conditionExpression xsi:type="tFormalExpression">${n &gt; 0}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="b_def" sourceRef="split" targetRef="b"/>
    <sequenceFlow id="ma" sourceRef="a" targetRef="merge"/>
    <sequenceFlow id="mb" sourceRef="b" targetRef="merge"/>
    <sequenceFlow id="mout" sourceRef="merge" targetRef="after">
    </sequenceFlow>
    <sequenceFlow id="f2" sourceRef="after" targetRef="e"/>
  </process>
</definitions>`)

func TestMergeExclusiveGateway(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "merge_gw", Data: mergeGatewayXML}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	// 高分支。
	inst, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "merge_gw", StarterID: "u0",
		Variables: flow.Vars{"n": float64(1)},
	})
	if err != nil {
		t.Fatalf("start hi: %v", err)
	}
	ta, _ := eng.ListPendingTasks(ctx, "t1", "ua")
	if len(ta) != 1 {
		t.Fatalf("branch a task missing")
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: ta[0].ID, ActorID: "ua", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete a: %v", err)
	}
	tz, _ := eng.ListPendingTasks(ctx, "t1", "uz")
	if len(tz) != 1 {
		t.Fatalf("after-merge task missing (hi path)")
	}
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tz[0].ID, ActorID: "uz", Outcome: flow.OutcomeApprove,
	})
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("hi path status = %s", inst.Status)
	}

	// 默认分支。
	inst2, _, _ := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "merge_gw", StarterID: "u0",
		Variables: flow.Vars{"n": float64(0)},
	})
	tb, _ := eng.ListPendingTasks(ctx, "t1", "ub")
	if len(tb) != 1 {
		t.Fatalf("branch b task missing")
	}
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tb[0].ID, ActorID: "ub", Outcome: flow.OutcomeApprove,
	})
	tz, _ = eng.ListPendingTasks(ctx, "t1", "uz")
	if len(tz) != 1 {
		t.Fatal("after-merge task missing (def path)")
	}
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tz[0].ID, ActorID: "uz", Outcome: flow.OutcomeApprove,
	})
	inst2, _ = eng.GetInstance(ctx, inst2.ID)
	if inst2.Status != flow.InstanceApproved {
		t.Fatalf("def path status = %s", inst2.Status)
	}
	_ = strings.TrimSpace("")
}
