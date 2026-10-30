package go_wind_flow_test

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	windjson "github.com/tx7do/go-wind-flow/format/windjson"
	"github.com/tx7do/go-wind-flow/store/memory"
)

// 组身份：fin 组内 2 人，其他用户无组。
type groupIdentity struct {
	StaticIdentity
}

func (groupIdentity) ResolveGroupsForUser(_ context.Context, _, userID string) ([]string, error) {
	switch userID {
	case "fin_a", "fin_b":
		return []string{"finance"}, nil
	}
	return nil, nil
}

func newGroupEngine(t *testing.T) *flow.Engine {
	t.Helper()
	return flow.New(
		flow.WithStore(memory.New()),
		flow.WithIdentity(groupIdentity{}),
		flow.WithImporters(bpmn.Importer{}, windjson.Importer{}),
	)
}

const groupFlowXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="grp" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="review" name="财务审核" candidateGroups="finance"/>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <sequenceFlow id="f2" sourceRef="review" targetRef="e"/>
  </process>
</definitions>`

func TestCandidateGroupLifecycle(t *testing.T) {
	eng := newGroupEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "grp", Data: []byte(groupFlowXML)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "grp", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// 组待办：组内两人可见，非组员不可见。
	for _, uid := range []string{"fin_a", "fin_b"} {
		tasks, err := eng.ListCandidateTasks(ctx, "t1", uid)
		if err != nil || len(tasks) != 1 {
			t.Fatalf("candidate tasks for %s = %d, %v", uid, len(tasks), err)
		}
	}
	if tasks, _ := eng.ListCandidateTasks(ctx, "t1", "outsider"); len(tasks) != 0 {
		t.Fatal("outsider should not see group task")
	}

	// 非组员认领 → 拒绝。
	if err := eng.ClaimTask(ctx, func() string {
		tasks, _ := eng.ListCandidateTasks(ctx, "t1", "fin_a")
		return tasks[0].ID
	}(), "outsider"); err == nil {
		t.Fatal("outsider claim should fail")
	}

	// 组员认领 → 成为个人待办；组待办消失；重复认领拒绝。
	tasks, _ := eng.ListCandidateTasks(ctx, "t1", "fin_a")
	taskID := tasks[0].ID
	if err := eng.ClaimTask(ctx, taskID, "fin_a"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if tasks, _ := eng.ListCandidateTasks(ctx, "t1", "fin_b"); len(tasks) != 0 {
		t.Fatal("claimed task should leave candidate pool")
	}
	if err := eng.ClaimTask(ctx, taskID, "fin_b"); err == nil {
		t.Fatal("double claim should fail")
	}

	// 退认 → 回池；再认领。
	if err := eng.UnclaimTask(ctx, taskID, "fin_a"); err != nil {
		t.Fatalf("unclaim: %v", err)
	}
	if tasks, _ := eng.ListCandidateTasks(ctx, "t1", "fin_b"); len(tasks) != 1 {
		t.Fatal("unclaimed task should be back in pool")
	}
	if err := eng.ClaimTask(ctx, taskID, "fin_b"); err != nil {
		t.Fatalf("re-claim: %v", err)
	}

	// 认领人办结 → APPROVED。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: taskID, ActorID: "fin_b", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s", inst.Status)
	}

	// 历史含认领事件。
	hist, _ := eng.ListHistory(ctx, inst.ID)
	found := false
	for _, h := range hist {
		if h.Type == flow.EventTaskClaimed {
			found = true
		}
	}
	if !found {
		t.Fatal("task.claimed event missing")
	}
}

func TestGroupPersistenceEntGorm(t *testing.T) {
	// 候选组字段的持久化在各自模块测试中覆盖（store/ent、store/gorm），
	// 这里防回归：内存 store 往返不丢组。
	st := memory.New()
	eng := flow.New(flow.WithStore(st), flow.WithIdentity(groupIdentity{}), flow.WithImporters(bpmn.Importer{}))
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "grp", Data: []byte(groupFlowXML)}); err != nil {
		t.Fatal(err)
	}
	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "grp", StarterID: "u0"})
	tasks, _ := eng.ListInstanceTasks(ctx, inst.ID)
	if tasks[0].Assignee != "" || len(tasks[0].CandidateGroups) != 1 || tasks[0].CandidateGroups[0] != "finance" {
		t.Fatalf("group task mapping wrong: %+v", tasks[0])
	}
}
