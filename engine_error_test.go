package go_wind_flow_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

// 主流程：start → call(子流程) ─┬─(错误边界 PAY)→ compensate → end
//                               └(正常)→ ok → end2
// 子流程：start → check(脚本失败抛错场景用 errorEnd 模拟) → errorEnd(PAY) → end
const errorParentXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="err_main" isExecutable="true">
    <startEvent id="s"/>
    <callActivity id="call_child" calledElement="err_child"/>
    <boundaryEvent id="pay_err" attachedToRef="call_child">
      <errorEventDefinition errorRef="PAY"/>
    </boundaryEvent>
    <userTask id="compensate" name="补偿处理"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"ops"}]}</flow:assignees></extensionElements></userTask>
    <userTask id="ok" name="正常归档"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"archiver"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e1"/>
    <endEvent id="e2"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="call_child"/>
    <sequenceFlow id="fb" sourceRef="pay_err" targetRef="compensate"/>
    <sequenceFlow id="f2" sourceRef="compensate" targetRef="e1"/>
    <sequenceFlow id="f3" sourceRef="call_child" targetRef="ok"/>
    <sequenceFlow id="f4" sourceRef="ok" targetRef="e2"/>
  </process>
</definitions>`

var errorChildXML = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="err_child" isExecutable="true">
    <startEvent id="cs"/>
    <userTask id="check" name="校验"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"STARTER"}]}</flow:assignees></extensionElements></userTask>
    <exclusiveGateway id="gw" default="to_end"/>
    <endEvent id="ce"/>
    <endEvent id="pay_fail">
      <errorEventDefinition errorRef="PAY"/>
    </endEvent>
    <sequenceFlow id="cf1" sourceRef="cs" targetRef="check"/>
    <sequenceFlow id="cf2" sourceRef="check" targetRef="gw"/>
    <sequenceFlow id="cf3" sourceRef="gw" targetRef="pay_fail">
      <conditionExpression xsi:type="tFormalExpression" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">${fail == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="to_end" sourceRef="gw" targetRef="ce"/>
  </process>
</definitions>`)

func completeChildCheck(t *testing.T, eng *flow.Engine, fail bool) {
	t.Helper()
	ctx := context.Background()
	abortedOrRunning, _ := eng.ListInstancesByStatus(ctx, "t1", flow.InstanceRunning)
	for _, i := range abortedOrRunning {
		if i.DefinitionKey != "err_child" {
			continue
		}
		tasks, _ := eng.ListPendingTasks(ctx, "t1", "u0")
		for _, x := range tasks {
			if x.InstanceID == i.ID {
				_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
					TaskID: x.ID, ActorID: "u0", Outcome: flow.OutcomeApprove,
					Variables: flow.Vars{"fail": fail},
				})
			}
		}
	}
}

func deployErrorFlow(t *testing.T, fail bool) (*flow.Engine, string) {
	t.Helper()
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "err_child", Data: []byte(errorChildXML)}); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "err_main", Data: []byte(errorParentXML)}); err != nil {
		t.Fatalf("deploy main: %v", err)
	}
	inst, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "err_main", StarterID: "u0",
		Variables: flow.Vars{"fail": fail},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return eng, inst.ID
}

func TestErrorPropagationCaught(t *testing.T) {
	eng, mainID := deployErrorFlow(t, true)
	ctx := context.Background()

	// 驱动子流程校验任务（fail 变量经继承变量驱动网关走向）。
	completeChildCheck(t, eng, true)

	// 子流程实例已 ABORTED（错误终结）；父实例 RUNNING，补偿任务出现。
	aborted, _ := eng.ListInstancesByStatus(ctx, "t1", flow.InstanceAborted)
	var childID string
	for _, i := range aborted {
		if i.DefinitionKey == "err_child" {
			childID = i.ID
		}
	}
	if childID == "" {
		t.Fatal("child instance missing")
	}
	child, _ := eng.GetInstance(ctx, childID)
	if child.Status != flow.InstanceAborted {
		t.Fatalf("child status = %s, want ABORTED", child.Status)
	}
	comp, _ := eng.ListPendingTasks(ctx, "t1", "ops")
	if len(comp) != 1 || comp[0].NodeID != "compensate" {
		t.Fatalf("compensate task wrong: %+v", comp)
	}

	// 补偿办结 → 父实例 APPROVED（沿错误分支走完）。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: comp[0].ID, ActorID: "ops", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete compensate: %v", err)
	}
	parent, _ := eng.GetInstance(ctx, mainID)
	if parent.Status != flow.InstanceApproved {
		t.Fatalf("parent status = %s, want APPROVED", parent.Status)
	}

	// 历史记录错误码。
	hist, _ := eng.ListHistory(ctx, childID)
	found := false
	for _, h := range hist {
		if v, ok := h.Payload["errorCode"]; ok && strings.Contains(v.(string), "PAY") {
			found = true
		}
	}
	if !found {
		t.Fatal("errorCode missing in child history")
	}
}

func TestErrorPropagationUncaught(t *testing.T) {
	// fail=false → 子流程正常走完 → 父走正常分支。先验证正常路径不受影响。
	eng, mainID := deployErrorFlow(t, false)
	ctx := context.Background()
	completeChildCheck(t, eng, false)
	arch, _ := eng.ListPendingTasks(ctx, "t1", "archiver")
	if len(arch) != 1 {
		t.Fatalf("normal path broken: %d", len(arch))
	}
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: arch[0].ID, ActorID: "archiver", Outcome: flow.OutcomeApprove,
	})
	parent, _ := eng.GetInstance(ctx, mainID)
	if parent.Status != flow.InstanceApproved {
		t.Fatalf("status = %s", parent.Status)
	}

	// 未捕获场景：无错误边界的父流程 → 子错误 → 父 REJECTED（fail-closed）。
	eng2 := newEngine(t)
	ctx2 := context.Background()
	noBoundaryXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="err_main_nb" isExecutable="true">
    <startEvent id="s"/>
    <callActivity id="call_child" calledElement="err_child"/>
    <userTask id="ok" name="正常归档"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"archiver"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e2"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="call_child"/>
    <sequenceFlow id="f3" sourceRef="call_child" targetRef="ok"/>
    <sequenceFlow id="f4" sourceRef="ok" targetRef="e2"/>
  </process>
</definitions>`)
	if _, err := eng2.Deploy(ctx2, flow.DeployRequest{TenantID: "t1", Key: "err_child", Data: errorChildXML}); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	if _, err := eng2.Deploy(ctx2, flow.DeployRequest{TenantID: "t1", Key: "err_main_nb", Data: noBoundaryXML}); err != nil {
		t.Fatalf("deploy no-boundary main: %v", err)
	}
	inst, _, err := eng2.Start(ctx2, flow.StartRequest{
		TenantID: "t1", Key: "err_main_nb", StarterID: "u0",
		Variables: flow.Vars{"fail": true},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	completeChildCheck(t, eng2, true)
	inst, _ = eng2.GetInstance(ctx2, inst.ID)
	if inst.Status != flow.InstanceRejected {
		t.Fatalf("uncaught error: parent status = %s, want REJECTED (fail-closed)", inst.Status)
	}
}
