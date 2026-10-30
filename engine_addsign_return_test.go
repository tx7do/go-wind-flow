package go_wind_flow_test

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

// 三级线性审批：u1 → u2 → u3（wind-json 驱动）。
func deployLinear(t *testing.T, eng *flow.Engine) {
	t.Helper()
	defJSON := []byte(`{
		"version": 2,
		"nodes": [
			{"id": "start", "type": "START"},
			{"id": "u1", "type": "TASK", "approvers": [{"type": "USER", "id": 11}]},
			{"id": "u2", "type": "TASK", "approvers": [{"type": "USER", "id": 22}]},
			{"id": "u3", "type": "TASK", "approvers": [{"type": "USER", "id": 33}]},
			{"id": "end", "type": "END"}
		],
		"edges": [
			{"from": "start", "to": "u1"},
			{"from": "u1", "to": "u2"},
			{"from": "u2", "to": "u3"},
			{"from": "u3", "to": "end"}
		]
	}`)
	if _, err := eng.Deploy(context.Background(), flow.DeployRequest{
		TenantID: "t1", Key: "linear", Data: defJSON,
	}); err != nil {
		t.Fatalf("deploy linear: %v", err)
	}
}

func pendingOf(t *testing.T, eng *flow.Engine, instanceID, assignee string) *flow.Task {
	t.Helper()
	tasks, err := eng.ListPendingTasks(context.Background(), "t1", assignee)
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	for _, tk := range tasks {
		if tk.InstanceID == instanceID {
			return tk
		}
	}
	t.Fatalf("no pending task for %s in instance %s", assignee, instanceID)
	return nil
}

func TestAddTaskApprovers(t *testing.T) {
	eng := newEngine(t)
	deployLinear(t, eng)
	ctx := context.Background()

	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "linear", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	task := pendingOf(t, eng, inst.ID, "11")

	// 加签两人 → 同节点两个新待办。
	created, err := eng.AddTaskApprovers(ctx, task.ID, "11", []string{"44", "55"})
	if err != nil || len(created) != 2 {
		t.Fatalf("AddTaskApprovers = %d, %v", len(created), err)
	}
	tasks, _ := eng.ListInstanceTasks(ctx, inst.ID)
	pending := 0
	for _, tk := range tasks {
		if tk.Status == flow.TaskPending {
			pending++
		}
	}
	if pending != 3 {
		t.Fatalf("pending after add-sign = %d, want 3 (11+44+55 会签)", pending)
	}

	// 重复加签同一人 → 去重，不新建。
	if _, err := eng.AddTaskApprovers(ctx, task.ID, "11", []string{"44"}); err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	tasks, _ = eng.ListInstanceTasks(ctx, inst.ID)
	pending = 0
	for _, tk := range tasks {
		if tk.Status == flow.TaskPending {
			pending++
		}
	}
	if pending != 3 {
		t.Fatalf("pending after duplicate add = %d, want 3", pending)
	}

	// 会签：全员通过才推进。
	for _, uid := range []string{"11", "44", "55"} {
		tk := pendingOf(t, eng, inst.ID, uid)
		if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
			TaskID: tk.ID, ActorID: uid, Outcome: flow.OutcomeApprove,
		}); err != nil {
			t.Fatalf("complete %s: %v", uid, err)
		}
	}
	next := pendingOf(t, eng, inst.ID, "22")
	if next.NodeID != "u2" {
		t.Fatalf("advanced to %s, want u2", next.NodeID)
	}
}

func TestReturnTo(t *testing.T) {
	eng := newEngine(t)
	deployLinear(t, eng)
	ctx := context.Background()

	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "linear", StarterID: "u0"})

	// 走到 u3。
	tk := pendingOf(t, eng, inst.ID, "11")
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{TaskID: tk.ID, ActorID: "11", Outcome: flow.OutcomeApprove})
	tk = pendingOf(t, eng, inst.ID, "22")
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{TaskID: tk.ID, ActorID: "22", Outcome: flow.OutcomeApprove})
	tk = pendingOf(t, eng, inst.ID, "33")

	// 非祖先节点 → 拒绝（u3 从 u2 不可达反向）。
	if err := eng.ReturnTo(ctx, tk.ID, "33", "u3", ""); err == nil {
		t.Fatal("return-to self should fail")
	}
	if err := eng.ReturnTo(ctx, tk.ID, "33", "nope", ""); err == nil {
		t.Fatal("return-to unknown node should fail")
	}

	// 退回 u1：待办取消、实例保持 RUNNING、u1 重新产生待办。
	if err := eng.ReturnTo(ctx, tk.ID, "33", "u1", "材料不全退回"); err != nil {
		t.Fatalf("ReturnTo: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceRunning {
		t.Fatalf("status after return = %s, want RUNNING", inst.Status)
	}
	again := pendingOf(t, eng, inst.ID, "11")
	if again.NodeID != "u1" {
		t.Fatalf("returned task node = %s, want u1", again.NodeID)
	}
	// 旧 u3 待办应已取消。
	oldTasks, _ := eng.ListInstanceTasks(ctx, inst.ID)
	for _, ot := range oldTasks {
		if ot.ID == tk.ID && ot.Status != flow.TaskCancelled {
			t.Fatalf("old task should be CANCELLED, got %s", ot.Status)
		}
	}

	// 重新走完全程 → APPROVED。
	for _, uid := range []string{"11", "22", "33"} {
		tk = pendingOf(t, eng, inst.ID, uid)
		if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
			TaskID: tk.ID, ActorID: uid, Outcome: flow.OutcomeApprove,
		}); err != nil {
			t.Fatalf("re-complete %s: %v", uid, err)
		}
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("final status = %s, want APPROVED", inst.Status)
	}

	// 历史包含退回事件。
	hist, _ := eng.ListHistory(ctx, inst.ID)
	found := false
	for _, h := range hist {
		if h.Type == flow.EventInstanceReturned {
			found = true
		}
	}
	if !found {
		t.Fatal("instance.returned event missing")
	}
}
