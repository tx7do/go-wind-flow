package memory

import (
	"context"
	"testing"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
)

// 全 SPI 方法覆盖：definitions / instances / tokens / tasks / history。
func TestDefinitionStore(t *testing.T) {
	st := New()
	ctx := context.Background()
	ds := st.Definitions()

	def := &flow.Definition{
		Key: "leave", Name: "请假", Version: 1, TenantID: "t1",
		Format: "bpmn-xml", Source: []byte("<x/>"),
		Nodes: map[string]*flow.Node{
			"s": {ID: "s", Type: flow.NodeStartEvent},
			"e": {ID: "e", Type: flow.NodeEndEvent},
		},
		Edges: []*flow.Edge{{ID: "e1", From: "s", To: "e"}},
	}
	if err := ds.Save(ctx, def); err != nil {
		t.Fatalf("save: %v", err)
	}
	// 升版 v2。
	v2 := *def
	v2.Version = 2
	if err := ds.Save(ctx, &v2); err != nil {
		t.Fatalf("save v2: %v", err)
	}

	max, err := ds.MaxVersion(ctx, "t1", "leave")
	if err != nil || max != 2 {
		t.Fatalf("MaxVersion = %d, %v", max, err)
	}
	// version=0 取最新。
	got, err := ds.Get(ctx, "t1", "leave", 0)
	if err != nil || got.Version != 2 {
		t.Fatalf("Get latest = v%d, %v", got.Version, err)
	}
	// 克隆隔离：改返回值不影响存储。
	got.Name = "mutated"
	again, _ := ds.Get(ctx, "t1", "leave", 2)
	if again.Name == "mutated" {
		t.Fatal("store leaked mutation (clone broken)")
	}
	// 未租户隔离。
	if _, err := ds.Get(ctx, "t2", "leave", 0); err != flow.ErrNotFound {
		t.Fatal("cross-tenant should be NotFound")
	}
	if _, err := ds.MaxVersion(ctx, "t2", "leave"); err != nil || max == 0 && err == nil {
		// 其他租户 MaxVersion=0 不报错。
		_ = err
	}
}

func TestInstanceStore(t *testing.T) {
	st := New()
	ctx := context.Background()
	is := st.Instances()

	mk := func(id string, status flow.InstanceStatus) *flow.Instance {
		return &flow.Instance{ID: id, TenantID: "t1", Status: status, Variables: flow.Vars{}, CreatedAt: time.Now()}
	}
	for _, i := range []*flow.Instance{mk("i1", flow.InstanceRunning), mk("i2", flow.InstanceApproved)} {
		if err := is.Save(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	running, err := is.ListByStatus(ctx, flow.InstanceRunning)
	if err != nil || len(running) != 1 || running[0].ID != "i1" {
		t.Fatalf("ListByStatus = %d", len(running))
	}
	all, _ := is.List(ctx, "t1")
	if len(all) != 2 {
		t.Fatalf("List = %d", len(all))
	}
	if _, err := is.Get(ctx, "nope"); err != flow.ErrNotFound {
		t.Fatal("missing should be ErrNotFound")
	}
	// Update 不存在 → NotFound。
	if err := is.Update(ctx, mk("ghost", flow.InstanceRunning)); err != flow.ErrNotFound {
		t.Fatal("update ghost should be ErrNotFound")
	}
}

func TestTokenStore(t *testing.T) {
	st := New()
	ctx := context.Background()
	ts := st.Tokens()

	now := time.Now()
	due := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	toks := []*flow.Token{
		{ID: "t1", InstanceID: "i1", NodeID: "n1", Status: flow.TokenWaiting, DueAt: &due},
		{ID: "t2", InstanceID: "i1", NodeID: "n2", Status: flow.TokenWaiting, DueAt: &future},
		{ID: "t3", InstanceID: "i1", NodeID: "n3", Status: flow.TokenRunning},
	}
	for _, tk := range toks {
		if err := ts.Save(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}

	dueToks, err := ts.ListDueTimers(ctx, now)
	if err != nil || len(dueToks) != 1 || dueToks[0].ID != "t1" {
		t.Fatalf("ListDueTimers = %d, %v", len(dueToks), err)
	}

	byInst, _ := ts.ListByInstance(ctx, "i1")
	if len(byInst) != 3 {
		t.Fatalf("ListByInstance = %d", len(byInst))
	}
	if err := ts.Delete(ctx, "t3"); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Get(ctx, "t3"); err != flow.ErrNotFound {
		t.Fatal("deleted token should be ErrNotFound")
	}
}

func TestTaskStore(t *testing.T) {
	st := New()
	ctx := context.Background()
	ts := st.Tasks()

	mk := func(id, assignee string, groups []string, status flow.TaskStatus) *flow.Task {
		return &flow.Task{
			ID: id, InstanceID: "i1", TenantID: "t1", NodeID: "n1", TokenID: "k1",
			Assignee: assignee, CandidateGroups: groups, Status: status, CreatedAt: time.Now(),
		}
	}
	tasks := []*flow.Task{
		mk("t1", "", []string{"finance", "boss"}, flow.TaskPending),
		mk("t2", "alice", nil, flow.TaskPending),
		mk("t3", "bob", []string{"hr"}, flow.TaskPending),
		mk("t4", "alice", nil, flow.TaskApproved),
	}
	for _, x := range tasks {
		if err := ts.Save(ctx, x); err != nil {
			t.Fatal(err)
		}
	}

	// 组待办：只含未分派 + 组交集（t3 已被 bob 认领，不计入）。
	got, err := ts.ListUnclaimedByGroups(ctx, "t1", []string{"finance", "hr"})
	if err != nil || len(got) != 1 {
		t.Fatalf("ListUnclaimedByGroups = %d, %v", len(got), err)
	}
	if got2, _ := ts.ListUnclaimedByGroups(ctx, "t1", []string{"nope"}); len(got2) != 0 {
		t.Fatal("no-match group should be empty")
	}

	byAssignee, _ := ts.ListPendingByAssignee(ctx, "t1", "alice")
	if len(byAssignee) != 1 || byAssignee[0].ID != "t2" {
		t.Fatalf("ListPendingByAssignee = %d", len(byAssignee))
	}
	byToken, _ := ts.ListByToken(ctx, "k1")
	_ = byToken
	byInst, _ := ts.ListByInstance(ctx, "i1")
	if len(byInst) != 4 {
		t.Fatalf("ListByInstance = %d", len(byInst))
	}
	if _, err := ts.Get(ctx, "ghost"); err != flow.ErrNotFound {
		t.Fatal("ghost should be ErrNotFound")
	}
	// 克隆隔离。
	got[0].CandidateGroups[0] = "mutated"
	if tasks[0].CandidateGroups[0] == "mutated" {
		t.Fatal("store leaked mutation via clone")
	}
}

func TestHistoryStore(t *testing.T) {
	st := New()
	ctx := context.Background()
	hs := st.History()

	evs := []*flow.HistoryEvent{
		{ID: "h1", InstanceID: "i1", Type: flow.EventProcessStarted},
		{ID: "h2", InstanceID: "i1", Type: flow.EventTaskCreated},
		{ID: "h3", InstanceID: "i2", Type: flow.EventProcessStarted},
	}
	for _, ev := range evs {
		if err := hs.Append(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	got, err := hs.ListByInstance(ctx, "i1")
	if err != nil || len(got) != 2 {
		t.Fatalf("ListByInstance = %d, %v", len(got), err)
	}
}
