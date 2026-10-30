package gormstore

import (
	"context"
	"testing"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
)

// TestDefinitionList 定义列举：按租户隔离，可按 key 过滤。
func TestDefinitionList(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	for _, d := range []*flow.Definition{
		sampleDef("t1", "leave", 1),
		sampleDef("t1", "leave", 2),
		sampleDef("t1", "expense", 1),
		sampleDef("t2", "leave", 1),
	} {
		if err := st.Definitions().Save(ctx, d); err != nil {
			t.Fatalf("save %s v%d: %v", d.Key, d.Version, err)
		}
	}

	all, err := st.Definitions().List(ctx, "t1", "")
	if err != nil || len(all) != 3 {
		t.Fatalf("List(t1) = %d, %v; want 3", len(all), err)
	}
	only, err := st.Definitions().List(ctx, "t1", "leave")
	if err != nil || len(only) != 2 {
		t.Fatalf("List(t1, leave) = %d, %v; want 2", len(only), err)
	}
	if only[0].Version != 1 || only[1].Version != 2 {
		t.Errorf("versions = %d,%d; want 1,2 (升序)", only[0].Version, only[1].Version)
	}
	if other, _ := st.Definitions().List(ctx, "t2", ""); len(other) != 1 {
		t.Errorf("List(t2) = %d, want 1（租户隔离）", len(other))
	}
	if empty, _ := st.Definitions().List(ctx, "t3", ""); len(empty) != 0 {
		t.Errorf("List(t3) = %d, want 0", len(empty))
	}
}

// TestInstanceListByStatus 实例按状态/租户列举。
func TestInstanceListByStatus(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	now := time.Now()
	for _, inst := range []*flow.Instance{
		{ID: "run-1", TenantID: "t1", DefinitionKey: "leave", Status: flow.InstanceRunning, Variables: flow.Vars{}, CreatedAt: now},
		{ID: "run-2", TenantID: "t1", DefinitionKey: "leave", Status: flow.InstanceRunning, Variables: flow.Vars{}, CreatedAt: now.Add(time.Second)},
		{ID: "done-1", TenantID: "t1", DefinitionKey: "leave", Status: flow.InstanceApproved, Variables: flow.Vars{}, CreatedAt: now},
		{ID: "run-3", TenantID: "t2", DefinitionKey: "leave", Status: flow.InstanceRunning, Variables: flow.Vars{}, CreatedAt: now},
	} {
		if err := st.Instances().Save(ctx, inst); err != nil {
			t.Fatalf("save %s: %v", inst.ID, err)
		}
	}

	running, err := st.Instances().ListByStatus(ctx, flow.InstanceRunning)
	if err != nil || len(running) != 3 {
		t.Fatalf("ListByStatus(RUNNING) = %d, %v; want 3", len(running), err)
	}
	// created_at 有并列（now），只断言时间最晚的排末尾。
	if running[len(running)-1].ID != "run-2" {
		t.Errorf("last = %s; want run-2 (created_at ASC)", running[len(running)-1].ID)
	}
	seen := map[string]bool{}
	for _, inst := range running {
		seen[inst.ID] = true
	}
	for _, want := range []string{"run-1", "run-2", "run-3"} {
		if !seen[want] {
			t.Errorf("running missing %s", want)
		}
	}
	approved, err := st.Instances().ListByStatus(ctx, flow.InstanceApproved)
	if err != nil || len(approved) != 1 || approved[0].ID != "done-1" {
		t.Fatalf("ListByStatus(APPROVED) = %+v, %v", approved, err)
	}
	t1, err := st.Instances().List(ctx, "t1")
	if err != nil || len(t1) != 3 {
		t.Fatalf("List(t1) = %d, %v; want 3", len(t1), err)
	}
	if _, err := st.Instances().Get(ctx, "ghost"); err != flow.ErrNotFound {
		t.Errorf("ghost instance err = %v, want ErrNotFound", err)
	}
}

// TestTokenCRUD 令牌增删查改与 Locals（变量作用域）持久化。
func TestTokenCRUD(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	now := time.Now()
	due := now.Add(-time.Minute)

	toks := []*flow.Token{
		{ID: "wait-1", InstanceID: "i1", NodeID: "timer", Status: flow.TokenWaiting, DueAt: &due, CreatedAt: now},
		{ID: "wait-2", InstanceID: "i1", NodeID: "timer", Status: flow.TokenWaiting,
			DueAt: ptrTime(now.Add(time.Hour)), CreatedAt: now},
		{ID: "run-1", InstanceID: "i1", NodeID: "task", Status: flow.TokenRunning,
			Locals: flow.Vars{"item": "u-7", "n": float64(2)}, CreatedAt: now},
		{ID: "other", InstanceID: "i2", NodeID: "timer", Status: flow.TokenWaiting, DueAt: &due, CreatedAt: now},
	}
	for _, tk := range toks {
		if err := st.Tokens().Save(ctx, tk); err != nil {
			t.Fatalf("save %s: %v", tk.ID, err)
		}
	}

	// 到期扫描：仅 WAITING 且 dueAt 已到；不含其它实例。
	expired, err := st.Tokens().ListDueTimers(ctx, now)
	if err != nil || len(expired) != 2 {
		t.Fatalf("ListDueTimers = %d, %v; want 2", len(expired), err)
	}

	// Locals 往返（变量作用域依赖，丢失会导致串行多实例变量不可读）。
	got, err := st.Tokens().Get(ctx, "run-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Locals["item"] != "u-7" || got.Locals["n"] != float64(2) {
		t.Errorf("Locals roundtrip broken: %+v", got.Locals)
	}

	// 更新：状态迁移落库。
	got.Status = flow.TokenEnded
	if err := st.Tokens().Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if again, _ := st.Tokens().Get(ctx, "run-1"); again.Status != flow.TokenEnded {
		t.Errorf("status = %s, want ENDED", again.Status)
	}

	byInst, err := st.Tokens().ListByInstance(ctx, "i1")
	if err != nil || len(byInst) != 3 {
		t.Fatalf("ListByInstance = %d, %v; want 3", len(byInst), err)
	}

	if err := st.Tokens().Delete(ctx, "other"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := st.Tokens().Get(ctx, "other"); err != flow.ErrNotFound {
		t.Errorf("deleted token err = %v, want ErrNotFound", err)
	}
}

// TestTaskGroupQueries 任务按实例/组列举。
func TestTaskGroupQueries(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	now := time.Now()

	tasks := []*flow.Task{
		{ID: "t1", InstanceID: "i1", TenantID: "t1", NodeID: "review", TokenID: "k1",
			CandidateGroups: []string{"finance"}, Status: flow.TaskPending, CreatedAt: now},
		{ID: "t2", InstanceID: "i1", TenantID: "t1", NodeID: "review", TokenID: "k2",
			CandidateGroups: []string{"hr"}, Status: flow.TaskPending, CreatedAt: now},
		{ID: "t3", InstanceID: "i1", TenantID: "t1", NodeID: "review", TokenID: "k3",
			Assignee: "bob", CandidateGroups: []string{"finance"}, Status: flow.TaskPending, CreatedAt: now},
		{ID: "t4", InstanceID: "i2", TenantID: "t1", NodeID: "review", TokenID: "k4",
			CandidateGroups: []string{"finance"}, Status: flow.TaskPending, CreatedAt: now},
	}
	for _, tk := range tasks {
		if err := st.Tasks().Save(ctx, tk); err != nil {
			t.Fatalf("save %s: %v", tk.ID, err)
		}
	}

	byInst, err := st.Tasks().ListByInstance(ctx, "i1")
	if err != nil || len(byInst) != 3 {
		t.Fatalf("ListByInstance = %d, %v; want 3", len(byInst), err)
	}

	// 组待办：只回未分派的；t3 已被 bob 认领，不计入。
	unclaimed, err := st.Tasks().ListUnclaimedByGroups(ctx, "t1", []string{"finance"})
	if err != nil || len(unclaimed) != 2 {
		t.Fatalf("ListUnclaimedByGroups = %d, %v; want 2（t1,t4）", len(unclaimed), err)
	}
	for _, tk := range unclaimed {
		if tk.ID == "t3" {
			t.Error("claimed task t3 must not appear in candidate list")
		}
	}
	// 无匹配组 → 空。
	if none, _ := st.Tasks().ListUnclaimedByGroups(ctx, "t1", []string{"legal"}); len(none) != 0 {
		t.Errorf("ListUnclaimedByGroups(legal) = %d, want 0", len(none))
	}
	// 跨租户隔离。
	if none, _ := st.Tasks().ListUnclaimedByGroups(ctx, "t2", []string{"finance"}); len(none) != 0 {
		t.Errorf("ListUnclaimedByGroups(t2) = %d, want 0", len(none))
	}

	if _, err := st.Tasks().Get(ctx, "ghost"); err != flow.ErrNotFound {
		t.Errorf("ghost task err = %v, want ErrNotFound", err)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
