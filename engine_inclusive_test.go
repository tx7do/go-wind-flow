package go_wind_flow_test

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

// inclusiveXML 包容网关：按条件选择分支，汇合按实际分叉数吸收。
//   - amount > 100  → 财务审
//   - urgent        → 法务审
//   - 都不命中      → 默认旁路
const inclusiveXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="inc" isExecutable="true">
    <startEvent id="s"/>
    <inclusiveGateway id="fork"/>
    <userTask id="fin"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"fin"}]}</flow:assignees></extensionElements></userTask>
    <userTask id="law"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"law"}]}</flow:assignees></extensionElements></userTask>
    <userTask id="bypass"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"clerk"}]}</flow:assignees></extensionElements></userTask>
    <inclusiveGateway id="join"/>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="fork"/>
    <sequenceFlow id="f2" sourceRef="fork" targetRef="fin"><conditionExpression>${amount > 100}</conditionExpression></sequenceFlow>
    <sequenceFlow id="f3" sourceRef="fork" targetRef="law"><conditionExpression>${urgent}</conditionExpression></sequenceFlow>
    <sequenceFlow id="f4" sourceRef="fork" targetRef="bypass"/>
    <sequenceFlow id="f5" sourceRef="fin" targetRef="join"/>
    <sequenceFlow id="f6" sourceRef="law" targetRef="join"/>
    <sequenceFlow id="f7" sourceRef="bypass" targetRef="join"/>
    <sequenceFlow id="f8" sourceRef="join" targetRef="e"/>
  </process>
</definitions>`

func newInclusiveEngine(t *testing.T) *flow.Engine {
	t.Helper()
	eng := newEngine(t)
	if _, err := eng.Deploy(context.Background(), flow.DeployRequest{
		TenantID: "t1", Key: "inc", Data: []byte(inclusiveXML),
	}); err != nil {
		t.Fatalf("deploy inclusive: %v", err)
	}
	return eng
}

// activeTasks 返回实例中待办任务的节点 ID 集合。
func activeTasks(t *testing.T, eng *flow.Engine, instanceID string) map[string]*flow.Task {
	t.Helper()
	tasks, err := eng.ListInstanceTasks(context.Background(), instanceID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	out := map[string]*flow.Task{}
	for _, tk := range tasks {
		if tk.Status == flow.TaskPending {
			out[tk.NodeID] = tk
		}
	}
	return out
}

// TestInclusiveGatewayBothBranches 两个条件都命中：分叉两支，汇合等 2 个令牌。
func TestInclusiveGatewayBothBranches(t *testing.T) {
	eng := newInclusiveEngine(t)
	ctx := context.Background()
	inst, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "inc", StarterID: "u0",
		Variables: flow.Vars{"amount": float64(500), "urgent": true},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	pending := activeTasks(t, eng, inst.ID)
	if len(pending) != 2 || pending["fin"] == nil || pending["law"] == nil {
		t.Fatalf("pending = %v, want fin+law", keys(pending))
	}
	// 只完成一支：汇合未满足，实例仍运行、也不产生下游任务。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: pending["fin"].ID, ActorID: "fin", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete fin: %v", err)
	}
	if got, _ := eng.GetInstance(ctx, inst.ID); got.Status != flow.InstanceRunning {
		t.Fatalf("status after one branch = %s, want RUNNING", got.Status)
	}
	// 第二支完成 → 汇合吸收 → APPROVED。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: pending["law"].ID, ActorID: "law", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete law: %v", err)
	}
	if got, _ := eng.GetInstance(ctx, inst.ID); got.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", got.Status)
	}
}

// TestInclusiveGatewaySingleBranch 仅一个条件命中：汇合按 joinExpected 只等 1 个令牌。
func TestInclusiveGatewaySingleBranch(t *testing.T) {
	eng := newInclusiveEngine(t)
	ctx := context.Background()
	inst, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "inc", StarterID: "u0",
		Variables: flow.Vars{"amount": float64(5), "urgent": true},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	pending := activeTasks(t, eng, inst.ID)
	if len(pending) != 1 || pending["law"] == nil {
		t.Fatalf("pending = %v, want only law", keys(pending))
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: pending["law"].ID, ActorID: "law", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got, _ := eng.GetInstance(ctx, inst.ID); got.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", got.Status)
	}
}

// TestInclusiveGatewayDefaultBranch 全不命中：走无条件默认边。
func TestInclusiveGatewayDefaultBranch(t *testing.T) {
	eng := newInclusiveEngine(t)
	ctx := context.Background()
	inst, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "inc", StarterID: "u0",
		Variables: flow.Vars{"amount": float64(5), "urgent": false},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	pending := activeTasks(t, eng, inst.ID)
	if len(pending) != 1 || pending["bypass"] == nil {
		t.Fatalf("pending = %v, want only bypass", keys(pending))
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: pending["bypass"].ID, ActorID: "clerk", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got, _ := eng.GetInstance(ctx, inst.ID); got.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", got.Status)
	}
}

// TestInclusiveGatewayNoDefault 无默认边且全不命中 → fail-closed 表达式错误。
func TestInclusiveGatewayNoDefault(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="inc2" isExecutable="true">
    <startEvent id="s"/>
    <inclusiveGateway id="fork"/>
    <userTask id="a"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"a"}]}</flow:assignees></extensionElements></userTask>
    <userTask id="b"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"b"}]}</flow:assignees></extensionElements></userTask>
    <inclusiveGateway id="join"/>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="fork"/>
    <sequenceFlow id="f2" sourceRef="fork" targetRef="a"><conditionExpression>${amount > 100}</conditionExpression></sequenceFlow>
    <sequenceFlow id="f3" sourceRef="fork" targetRef="b"><conditionExpression>${urgent}</conditionExpression></sequenceFlow>
    <sequenceFlow id="f4" sourceRef="a" targetRef="join"/>
    <sequenceFlow id="f5" sourceRef="b" targetRef="join"/>
    <sequenceFlow id="f6" sourceRef="join" targetRef="e"/>
  </process>
</definitions>`
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "inc2", Data: []byte(xml)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	_, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "inc2", StarterID: "u0",
		Variables: flow.Vars{"amount": float64(1), "urgent": false},
	})
	if err == nil {
		t.Fatal("no branch matched without default should fail closed")
	}
}

// TestInclusiveGatewayConditionError 条件求值失败 → fail-closed，不推进。
func TestInclusiveGatewayConditionError(t *testing.T) {
	eng := newInclusiveEngine(t)
	ctx := context.Background()
	// amount 缺失 → `amount > 100` 求值失败。
	_, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "inc", StarterID: "u0",
		Variables: flow.Vars{"urgent": false},
	})
	if err == nil {
		t.Fatal("expression error should fail closed")
	}
}

func keys(m map[string]*flow.Task) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
