package go_wind_flow_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
	"github.com/tx7do/go-wind-flow/store/memory"

	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	windjson "github.com/tx7do/go-wind-flow/format/windjson"
)

// StaticIdentity 测试用静态身份解析：LEADER → boss，其余直取。
type StaticIdentity struct{}

func (StaticIdentity) ResolveApprovers(_ context.Context, req flow.ResolveRequest) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, r := range req.Rules {
		var v string
		switch r.Kind {
		case flow.ApproverUser:
			v = r.Value
		case flow.ApproverStarter:
			v = req.StarterID
		case flow.ApproverLeader:
			v = "boss"
		}
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out, nil
}

func (StaticIdentity) ResolveDelegate(_ context.Context, _, userID string) (string, error) {
	return userID, nil
}

func newEngine(t *testing.T) *flow.Engine {
	t.Helper()
	return flow.New(
		flow.WithStore(memory.New()),
		flow.WithIdentity(StaticIdentity{}),
		flow.WithImporters(bpmn.Importer{}, windjson.Importer{}),
	)
}

func mustDeploy(t *testing.T, eng *flow.Engine, key, file string) *flow.Definition {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	def, err := eng.Deploy(context.Background(), flow.DeployRequest{
		TenantID: "t1", Key: key, Name: key, Data: data,
	})
	if err != nil {
		t.Fatalf("deploy %s: %v", key, err)
	}
	return def
}

func TestLeaveApproval_LongLeave(t *testing.T) {
	eng := newEngine(t)
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")

	ctx := context.Background()
	inst, tasks, err := eng.Start(ctx, flow.StartRequest{
		TenantID:  "t1",
		Key:       "leave_approval",
		StarterID: "u_emp",
		Variables: flow.Vars{"days": float64(5)},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if inst.Status != flow.InstanceRunning {
		t.Fatalf("instance status = %s, want RUNNING", inst.Status)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1 (leader review)", len(tasks))
	}
	task, _ := eng.ListInstanceTasks(ctx, inst.ID)
	if task[0].Assignee != "boss" {
		t.Fatalf("assignee = %s, want boss", task[0].Assignee)
	}

	// 直属领导通过 → days>3 进入总监审批（或签：单人任务）。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "boss", Outcome: flow.OutcomeApprove, Comment: "ok",
	}); err != nil {
		t.Fatalf("complete leader task: %v", err)
	}
	tasks2, _ := eng.ListPendingTasks(ctx, "t1", "u_1001")
	if len(tasks2) != 1 {
		t.Fatalf("director tasks = %d, want 1", len(tasks2))
	}

	// 总监通过 → 实例 APPROVED。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks2[0].ID, ActorID: "u_1001", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete director task: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("instance status = %s, want APPROVED", inst.Status)
	}
}

func TestLeaveApproval_ShortLeave_DefaultEdge(t *testing.T) {
	eng := newEngine(t)
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")

	ctx := context.Background()
	inst, tasks, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "leave_approval", StarterID: "u_emp",
		Variables: flow.Vars{"days": float64(1)},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "boss", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED (default edge)", inst.Status)
	}
}

func TestReject_FailClosed(t *testing.T) {
	eng := newEngine(t)
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")

	ctx := context.Background()
	inst, tasks, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "leave_approval", StarterID: "u_emp",
		Variables: flow.Vars{"days": float64(2)},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "other_user", Outcome: flow.OutcomeApprove,
	}); err == nil {
		t.Fatal("complete by non-assignee should fail")
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "boss", Outcome: flow.OutcomeReject,
	}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceRejected {
		t.Fatalf("status = %s, want REJECTED", inst.Status)
	}
}

func TestCounterSign_ALL_ThenAdvance(t *testing.T) {
	defJSON := []byte(`{
		"version": 2,
		"nodes": [
			{"id": "start", "type": "START"},
			{"id": "sign", "type": "TASK", "approvers": [
				{"type": "USER", "id": 101},
				{"type": "USER", "id": 102},
				{"type": "USER", "id": 103}
			], "strategy": "ALL"},
			{"id": "end", "type": "END"}
		],
		"edges": [
			{"from": "start", "to": "sign"},
			{"from": "sign", "to": "end"}
		]
	}`)
	eng := newEngine(t)
	if _, err := eng.Deploy(context.Background(), flow.DeployRequest{
		TenantID: "t1", Key: "cs", Data: defJSON,
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	ctx := context.Background()
	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "cs", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	tasks, _ := eng.ListInstanceTasks(ctx, inst.ID)
	if len(tasks) != 3 {
		t.Fatalf("tasks = %d, want 3 (会签)", len(tasks))
	}

	// 通过 2/3，实例仍运行。
	for i := 0; i < 2; i++ {
		if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
			TaskID: tasks[i].ID, ActorID: tasks[i].Assignee, Outcome: flow.OutcomeApprove,
		}); err != nil {
			t.Fatalf("complete %d: %v", i, err)
		}
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceRunning {
		t.Fatalf("status after 2/3 = %s, want RUNNING", inst.Status)
	}
	// 第 3 人通过 → APPROVED。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[2].ID, ActorID: tasks[2].Assignee, Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete 3: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}
}

func TestAnySign_FirstApproveCancelsOthers(t *testing.T) {
	defJSON := []byte(`{
		"version": 2,
		"nodes": [
			{"id": "start", "type": "START"},
			{"id": "sign", "type": "TASK", "approvers": [
				{"type": "USER", "id": 201},
				{"type": "USER", "id": 202}
			], "strategy": "ANY"},
			{"id": "end", "type": "END"}
		],
		"edges": [{"from": "start", "to": "sign"}, {"from": "sign", "to": "end"}]
	}`)
	eng := newEngine(t)
	if _, err := eng.Deploy(context.Background(), flow.DeployRequest{
		TenantID: "t1", Key: "any", Data: defJSON,
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	ctx := context.Background()
	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "any", StarterID: "u0"})
	tasks, _ := eng.ListInstanceTasks(ctx, inst.ID)
	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (或签)", len(tasks))
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0].ID, ActorID: tasks[0].Assignee, Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	rest, _ := eng.ListInstanceTasks(ctx, inst.ID)
	for _, r := range rest {
		if r.ID == tasks[1].ID && r.Status != flow.TaskCancelled {
			t.Fatalf("sibling task should be CANCELLED, got %s", r.Status)
		}
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}
}

func TestParallelForkJoin(t *testing.T) {
	defXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="parallel" name="并行会审" isExecutable="true">
    <startEvent id="s"/>
    <parallelGateway id="fork"/>
    <userTask id="a" name="财务审"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"fin"}]}</flow:assignees></extensionElements></userTask>
    <userTask id="b" name="法务审"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"law"}]}</flow:assignees></extensionElements></userTask>
    <parallelGateway id="join"/>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="fork"/>
    <sequenceFlow id="f2" sourceRef="fork" targetRef="a"/>
    <sequenceFlow id="f3" sourceRef="fork" targetRef="b"/>
    <sequenceFlow id="f4" sourceRef="a" targetRef="join"/>
    <sequenceFlow id="f5" sourceRef="b" targetRef="join"/>
    <sequenceFlow id="f6" sourceRef="join" targetRef="e"/>
  </process>
</definitions>`)
	eng := newEngine(t)
	if _, err := eng.Deploy(context.Background(), flow.DeployRequest{
		TenantID: "t1", Key: "pp", Data: defXML,
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	ctx := context.Background()
	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "pp", StarterID: "u0"})
	tasks, _ := eng.ListInstanceTasks(ctx, inst.ID)
	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (并行分支)", len(tasks))
	}
	// 只完成一支，实例仍运行。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0].ID, ActorID: tasks[0].Assignee, Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete branch 1: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceRunning {
		t.Fatalf("status after 1 branch = %s, want RUNNING", inst.Status)
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[1].ID, ActorID: tasks[1].Assignee, Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete branch 2: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}
}

// scriptStub 测试用 ScriptRunner 替身：约定 code=="calc" 时产出 score 变量。
type scriptStub struct{}

func (scriptStub) Run(_ context.Context, spec *flow.ScriptSpec, _ flow.Vars) (any, error) {
	if spec != nil && spec.Code == "calc" {
		return map[string]any{"score": float64(88)}, nil
	}
	return nil, nil
}

func TestScriptTask_And_Subprocess(t *testing.T) {
	eng := flow.New(
		flow.WithStore(memory.New()),
		flow.WithIdentity(StaticIdentity{}),
		flow.WithScripts(scriptStub{}),
		flow.WithImporters(bpmn.Importer{}),
	)
	ctx := context.Background()

	// 子流程定义。
	childXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="child" isExecutable="true">
    <startEvent id="s"/>
    <scriptTask id="calc" scriptFormat="js"><script>calc</script></scriptTask>
    <userTask id="c_review"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"child_reviewer"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="calc"/>
    <sequenceFlow id="f2" sourceRef="calc" targetRef="c_review"/>
    <sequenceFlow id="f3" sourceRef="c_review" targetRef="e"/>
  </process>
</definitions>`)
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "child", Data: childXML}); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	// 主流程：脚本任务 + 子流程。
	mainXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="main" isExecutable="true">
    <startEvent id="s"/>
    <callActivity id="call_child" calledElement="child"/>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="call_child"/>
    <sequenceFlow id="f2" sourceRef="call_child" targetRef="e"/>
  </process>
</definitions>`)
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "main", Data: mainXML}); err != nil {
		t.Fatalf("deploy main: %v", err)
	}

	// 无 ScriptRunner 时脚本任务 fail-closed（ErrNotConfigured）。
	bare := newEngine(t)
	if _, err := bare.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "child", Data: childXML}); err != nil {
		t.Fatalf("deploy child(bare): %v", err)
	}
	if _, _, err := bare.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "child", StarterID: "u0"}); err == nil {
		t.Fatal("script task without ScriptRunner should fail")
	}

	// 子流程完整走通后父实例 APPROVED。
	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "main", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start main: %v", err)
	}
	childTasks, _ := eng.ListPendingTasks(ctx, "t1", "child_reviewer")
	if len(childTasks) != 1 {
		t.Fatalf("child tasks = %d, want 1", len(childTasks))
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: childTasks[0].ID, ActorID: "child_reviewer", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete child task: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("parent status = %s, want APPROVED", inst.Status)
	}
}

func TestWithdraw(t *testing.T) {
	eng := newEngine(t)
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")
	ctx := context.Background()
	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "leave_approval", StarterID: "u_emp"})
	if err := eng.Withdraw(ctx, inst.ID, "not_starter"); err == nil {
		t.Fatal("non-starter withdraw should fail")
	}
	if err := eng.Withdraw(ctx, inst.ID, "u_emp"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceWithdrawn {
		t.Fatalf("status = %s, want WITHDRAWN", inst.Status)
	}
}

func TestDueTasks(t *testing.T) {
	defXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="due" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="t1"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"a1"}],"due":"1ns"}</flow:assignees></extensionElements></userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="t1"/>
    <sequenceFlow id="f2" sourceRef="t1" targetRef="e"/>
  </process>
</definitions>`)
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "due", Data: defXML}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	_, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "due", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	due, err := eng.CheckDueTasks(ctx, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("check due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("due tasks = %d, want 1", len(due))
	}
}
