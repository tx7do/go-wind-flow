package gormstore

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite" // 纯 Go sqlite 方言（免 CGO），仅测试用
	"gorm.io/gorm"

	"github.com/tx7do/go-wind-flow/flow"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st := New(db)
	if err := st.AutoMigrate(); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return st
}

func sampleDef(tenant, key string, version int) *flow.Definition {
	return &flow.Definition{
		Key: key, Name: key + "-name", Version: version, TenantID: tenant,
		Format: "bpmn-xml", Source: []byte("<definitions/>"), CreatedAt: time.Now(),
		Nodes: map[string]*flow.Node{
			"start": {ID: "start", Type: flow.NodeStartEvent},
			"end":   {ID: "end", Type: flow.NodeEndEvent},
		},
		Edges: []*flow.Edge{{ID: "e1", From: "start", To: "end"}},
	}
}

func TestDefinitionUpsertAndVersions(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	if err := st.Definitions().Save(ctx, sampleDef("t1", "leave", 1)); err != nil {
		t.Fatalf("save v1: %v", err)
	}
	if err := st.Definitions().Save(ctx, sampleDef("t1", "leave", 2)); err != nil {
		t.Fatalf("save v2: %v", err)
	}
	if err := st.Definitions().Save(ctx, sampleDef("t1", "leave", 1)); err != nil {
		t.Fatalf("resave v1 (upsert): %v", err)
	}

	maxV, err := st.Definitions().MaxVersion(ctx, "t1", "leave")
	if err != nil || maxV != 2 {
		t.Fatalf("MaxVersion = %d, %v; want 2", maxV, err)
	}
	latest, err := st.Definitions().Get(ctx, "t1", "leave", 0)
	if err != nil || latest.Version != 2 {
		t.Fatalf("Get latest = v%d, %v; want v2", latest.Version, err)
	}
	v1, err := st.Definitions().Get(ctx, "t1", "leave", 1)
	if err != nil || v1.Nodes["start"] == nil || len(v1.Edges) != 1 {
		t.Fatalf("Get v1 JSON roundtrip broken: %+v, %v", v1, err)
	}
	if _, err := st.Definitions().Get(ctx, "t1", "nope", 0); err != flow.ErrNotFound {
		t.Fatalf("missing def should be ErrNotFound, got %v", err)
	}
}

func TestInstanceTokenTaskRoundtrip(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	now := time.Now()

	inst := &flow.Instance{
		ID: "i1", TenantID: "t1", DefinitionKey: "leave", DefinitionVersion: 1,
		StarterID: "u1", Status: flow.InstanceRunning, Variables: flow.Vars{"days": float64(5)},
		CreatedAt: now,
	}
	if err := st.Instances().Save(ctx, inst); err != nil {
		t.Fatalf("save instance: %v", err)
	}
	if err := st.Tokens().Save(ctx, &flow.Token{
		ID: "k1", InstanceID: "i1", NodeID: "review", Status: flow.TokenWaiting, CreatedAt: now,
	}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	due := now.Add(-time.Hour)
	if err := st.Tasks().Save(ctx, &flow.Task{
		ID: "tk1", InstanceID: "i1", TenantID: "t1", NodeID: "review", TokenID: "k1",
		Assignee: "boss", Status: flow.TaskPending, CreatedAt: now, DueAt: &due,
	}); err != nil {
		t.Fatalf("save task: %v", err)
	}
	if err := st.Tasks().Save(ctx, &flow.Task{
		ID: "tk2", InstanceID: "i1", TenantID: "t1", NodeID: "review", TokenID: "k1",
		Assignee: "chief", Status: flow.TaskPending, CreatedAt: now,
	}); err != nil {
		t.Fatalf("save task2: %v", err)
	}

	pending, err := st.Tasks().ListPendingByAssignee(ctx, "t1", "boss")
	if err != nil || len(pending) != 1 {
		t.Fatalf("ListPendingByAssignee = %d, %v", len(pending), err)
	}
	dueTasks, err := st.Tasks().ListDue(ctx, time.Now())
	if err != nil || len(dueTasks) != 1 || dueTasks[0].ID != "tk1" {
		t.Fatalf("ListDue = %d, %v", len(dueTasks), err)
	}
	byToken, err := st.Tasks().ListByToken(ctx, "k1")
	if err != nil || len(byToken) != 2 {
		t.Fatalf("ListByToken = %d, %v", len(byToken), err)
	}

	tk1, _ := st.Tasks().Get(ctx, "tk1")
	tk1.Status = flow.TaskApproved
	tk1.EndedAt = &now
	if err := st.Tasks().Update(ctx, tk1); err != nil {
		t.Fatalf("update task: %v", err)
	}
	end := now
	inst.Status = flow.InstanceApproved
	inst.EndedAt = &end
	inst.Variables["result"] = "ok"
	if err := st.Instances().Update(ctx, inst); err != nil {
		t.Fatalf("update instance: %v", err)
	}

	got, _ := st.Instances().Get(ctx, "i1")
	if got.Status != flow.InstanceApproved || got.EndedAt == nil || got.Variables["result"] != "ok" {
		t.Fatalf("instance roundtrip broken: %+v", got)
	}
	tokens, _ := st.Tokens().ListByInstance(ctx, "i1")
	if len(tokens) != 1 || tokens[0].Status != flow.TokenWaiting {
		t.Fatalf("tokens roundtrip broken: %+v", tokens)
	}
}

func TestHistoryAndWithTx(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	err := st.WithTx(ctx, func(ctx context.Context) error {
		if err := st.Instances().Save(ctx, &flow.Instance{
			ID: "i-tx", TenantID: "t1", DefinitionKey: "k", Status: flow.InstanceRunning,
			Variables: flow.Vars{}, CreatedAt: time.Now(),
		}); err != nil {
			return err
		}
		return st.History().Append(ctx, &flow.HistoryEvent{
			ID: "h1", InstanceID: "i-tx", Type: flow.EventProcessStarted, CreatedAt: time.Now(),
		})
	})
	if err != nil {
		t.Fatalf("WithTx commit: %v", err)
	}
	events, _ := st.History().ListByInstance(ctx, "i-tx")
	if len(events) != 1 {
		t.Fatalf("history after commit = %d, want 1", len(events))
	}

	err = st.WithTx(ctx, func(ctx context.Context) error {
		if err := st.Instances().Save(ctx, &flow.Instance{
			ID: "i-rollback", TenantID: "t1", DefinitionKey: "k", Status: flow.InstanceRunning,
			Variables: flow.Vars{}, CreatedAt: time.Now(),
		}); err != nil {
			return err
		}
		return flow.ErrForbidden
	})
	if err == nil {
		t.Fatal("WithTx should propagate fn error")
	}
	if _, err := st.Instances().Get(ctx, "i-rollback"); err != flow.ErrNotFound {
		t.Fatalf("rollback instance should be absent, got %v", err)
	}

	err = st.WithTx(ctx, func(ctx context.Context) error {
		return st.WithTx(ctx, func(ctx context.Context) error {
			return st.History().Append(ctx, &flow.HistoryEvent{
				ID: "h2", InstanceID: "i-tx", Type: flow.EventTokenMoved, CreatedAt: time.Now(),
			})
		})
	})
	if err != nil {
		t.Fatalf("nested WithTx: %v", err)
	}
	events, _ = st.History().ListByInstance(ctx, "i-tx")
	if len(events) != 2 {
		t.Fatalf("history after nested WithTx = %d, want 2", len(events))
	}
}
