// Package entstore 提供 flow.Store 的 ent 实现（生产推荐）。
//
// 事务模型（架构文档 §3 的方案 1）：宿主在引擎入口用 WithTx 包裹，
// 一次引擎调用内的全部落库共享一个事务：
//
//	_ = store.WithTx(ctx, func(ctx context.Context) error {
//	    return eng.CompleteTask(ctx, req)
//	})
//
// 核心引擎对此无感知——WithTx 不在 flow.Store 接口上，是本实现的具体方法；
// ctx 中已有事务时直接复用（嵌套安全）。
//
// codegen：生成代码已提交进仓；schema 变更后需重新生成。
// 注意 entgo v0.14 的 loader 与 Go 1.27 不兼容，生成时请用 1.25 工具链：
//
//	GOSUMDB=sum.golang.org GOTOOLCHAIN=go1.25.0 go run entgo.io/ent/cmd/ent generate ./ent/schema
package entstore

import (
	"context"
	"fmt"
	"time"

	gen "github.com/tx7do/go-wind-flow/store/ent/ent"
	"github.com/tx7do/go-wind-flow/store/ent/ent/definition"
	"github.com/tx7do/go-wind-flow/store/ent/ent/predicate"
	"github.com/tx7do/go-wind-flow/store/ent/ent/historyevent"
	"github.com/tx7do/go-wind-flow/store/ent/ent/instance"
	"github.com/tx7do/go-wind-flow/store/ent/ent/task"
	"github.com/tx7do/go-wind-flow/store/ent/ent/token"

	"github.com/tx7do/go-wind-flow/flow"
)

// Store ent 实现。构造：entstore.New(client)；建表用 client.Schema.Create(ctx)
// 或接入宿主的版本化迁移（go-wind-admin 惯例）。
type Store struct {
	client *gen.Client
}

func New(client *gen.Client) *Store { return &Store{client: client} }

// ============================================================
// 事务
// ============================================================

type txKey struct{}

func (s *Store) WithTx(ctx context.Context, fn func(context.Context) error) error {
	if tx, ok := ctx.Value(txKey{}).(*gen.Tx); ok && tx != nil {
		return fn(ctx) // 已在外层事务内，复用
	}
	ttx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("flow/entstore: begin tx: %w", err)
	}
	defer func() {
		if v := recover(); v != nil {
			_ = ttx.Rollback()
			panic(v)
		}
	}()
	ctx2 := context.WithValue(ctx, txKey{}, ttx)
	if err := fn(ctx2); err != nil {
		if rbErr := ttx.Rollback(); rbErr != nil {
			err = fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
		}
		return err
	}
	return ttx.Commit()
}

// clientOf 返回当前 ctx 绑定的执行客户端（事务内返回事务客户端）。
func (s *Store) clientOf(ctx context.Context) *gen.Client {
	if tx, ok := ctx.Value(txKey{}).(*gen.Tx); ok && tx != nil {
		return tx.Client()
	}
	return s.client
}

// ============================================================
// DefinitionStore
// ============================================================

func defID(tenant, key string, version int) string {
	return tenant + "|" + key + "|" + itoa(version)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

type defStore struct{ s *Store }

func (s *Store) Definitions() flow.DefinitionStore { return defStore{s} }

func (d defStore) Save(ctx context.Context, def *flow.Definition) error {
	id := defID(def.TenantID, def.Key, def.Version)
	c := d.s.clientOf(ctx)
	exists, err := c.Definition.Query().Where(definition.IDEQ(id)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		_, err = c.Definition.UpdateOneID(id).
			SetName(def.Name).
			SetFormat(def.Format).
			SetSource(string(def.Source)).
			SetNodes(def.Nodes).
			SetEdges(def.Edges).
			SetCreatedAt(def.CreatedAt).
			Save(ctx)
		return err
	}
	_, err = c.Definition.Create().
		SetID(id).
		SetTenantID(def.TenantID).
		SetDefKey(def.Key).
		SetName(def.Name).
		SetVersion(def.Version).
		SetFormat(def.Format).
		SetSource(string(def.Source)).
		SetNodes(def.Nodes).
		SetEdges(def.Edges).
		SetCreatedAt(def.CreatedAt).
		Save(ctx)
	return err
}

func defToFlow(e *gen.Definition) *flow.Definition {
	return &flow.Definition{
		Key:       e.DefKey,
		Name:      e.Name,
		Version:   e.Version,
		TenantID:  e.TenantID,
		Format:    e.Format,
		Source:    []byte(e.Source),
		CreatedAt: e.CreatedAt,
		Nodes:     e.Nodes,
		Edges:     e.Edges,
	}
}

func (d defStore) Get(ctx context.Context, tenantID, key string, version int) (*flow.Definition, error) {
	c := d.s.clientOf(ctx)
	q := c.Definition.Query().Where(definition.TenantIDEQ(tenantID), definition.DefKeyEQ(key))
	if version > 0 {
		q = q.Where(definition.VersionEQ(version))
	}
	e, err := q.Order(gen.Desc("version")).First(ctx)
	if gen.IsNotFound(err) {
		return nil, flow.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return defToFlow(e), nil
}

func (d defStore) MaxVersion(ctx context.Context, tenantID, key string) (int, error) {
	e, err := d.s.clientOf(ctx).Definition.Query().
		Where(definition.TenantIDEQ(tenantID), definition.DefKeyEQ(key)).
		Order(gen.Desc("version")).
		First(ctx)
	if gen.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return e.Version, nil
}

func (d defStore) List(ctx context.Context, tenantID, key string) ([]*flow.Definition, error) {
	pred := []predicate.Definition{definition.TenantIDEQ(tenantID)}
	if key != "" {
		pred = append(pred, definition.DefKeyEQ(key))
	}
	es, err := d.s.clientOf(ctx).Definition.Query().
		Where(pred...).
		Order(gen.Asc("def_key"), gen.Asc("version")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*flow.Definition, 0, len(es))
	for _, e := range es {
		out = append(out, defToFlow(e))
	}
	return out, nil
}

// ============================================================
// InstanceStore
// ============================================================

type instStore struct{ s *Store }

func (s *Store) Instances() flow.InstanceStore { return instStore{s} }

func (i instStore) upsert(ctx context.Context, inst *flow.Instance) error {
	c := i.s.clientOf(ctx)
	exists, err := c.Instance.Query().Where(instance.IDEQ(inst.ID)).Exist(ctx)
	if err != nil {
		return err
	}
	apply := func(ic *gen.InstanceCreate) *gen.InstanceCreate {
		return ic.
			SetTenantID(inst.TenantID).
			SetDefinitionKey(inst.DefinitionKey).
			SetDefinitionVersion(inst.DefinitionVersion).
			SetBusinessKey(inst.BusinessKey).
			SetBusinessType(inst.BusinessType).
			SetStarterID(inst.StarterID).
			SetStatus(string(inst.Status)).
			SetVariables(inst.Variables).
			SetParentInstanceID(inst.ParentInstanceID).
			SetParentTokenID(inst.ParentTokenID).
			SetCreatedAt(inst.CreatedAt).
			SetNillableEndedAt(inst.EndedAt)
	}
	if exists {
		_, err = c.Instance.UpdateOneID(inst.ID).
			SetStatus(string(inst.Status)).
			SetVariables(inst.Variables).
			SetNillableEndedAt(inst.EndedAt).
			Save(ctx)
		return err
	}
	_, err = apply(c.Instance.Create().SetID(inst.ID)).Save(ctx)
	return err
}

func (i instStore) Save(ctx context.Context, inst *flow.Instance) error {
	return i.upsert(ctx, inst)
}

func (i instStore) Update(ctx context.Context, inst *flow.Instance) error {
	return i.upsert(ctx, inst)
}

func instToFlow(e *gen.Instance) *flow.Instance {
	return &flow.Instance{
		ID:                e.ID,
		TenantID:          e.TenantID,
		DefinitionKey:     e.DefinitionKey,
		DefinitionVersion: e.DefinitionVersion,
		BusinessKey:       e.BusinessKey,
		BusinessType:      e.BusinessType,
		StarterID:         e.StarterID,
		Status:            flow.InstanceStatus(e.Status),
		Variables:         e.Variables,
		ParentInstanceID:  e.ParentInstanceID,
		ParentTokenID:     e.ParentTokenID,
		CreatedAt:         e.CreatedAt,
		EndedAt:           e.EndedAt,
	}
}

func (i instStore) Get(ctx context.Context, id string) (*flow.Instance, error) {
	e, err := i.s.clientOf(ctx).Instance.Get(ctx, id)
	if gen.IsNotFound(err) {
		return nil, flow.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return instToFlow(e), nil
}

func (i instStore) ListByStatus(ctx context.Context, status flow.InstanceStatus) ([]*flow.Instance, error) {
	es, err := i.s.clientOf(ctx).Instance.Query().
		Where(instance.StatusEQ(string(status))).
		Order(gen.Asc("created_at")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*flow.Instance, 0, len(es))
	for _, e := range es {
		out = append(out, instToFlow(e))
	}
	return out, nil
}

func (i instStore) List(ctx context.Context, tenantID string) ([]*flow.Instance, error) {
	es, err := i.s.clientOf(ctx).Instance.Query().
		Where(instance.TenantIDEQ(tenantID)).
		Order(gen.Asc("created_at")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*flow.Instance, 0, len(es))
	for _, e := range es {
		out = append(out, instToFlow(e))
	}
	return out, nil
}

// ============================================================
// TokenStore
// ============================================================

type tokenStore struct{ s *Store }

func (s *Store) Tokens() flow.TokenStore { return tokenStore{s} }

func (t tokenStore) Save(ctx context.Context, tk *flow.Token) error {
	return t.upsert(ctx, tk)
}

func (t tokenStore) Update(ctx context.Context, tk *flow.Token) error {
	return t.upsert(ctx, tk)
}

func (t tokenStore) upsert(ctx context.Context, tk *flow.Token) error {
	c := t.s.clientOf(ctx)
	exists, err := c.Token.Query().Where(token.IDEQ(tk.ID)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		_, err = c.Token.UpdateOneID(tk.ID).
			SetNodeID(tk.NodeID).
			SetStatus(string(tk.Status)).
			SetParentID(tk.ParentID).
			SetChildInstID(tk.ChildInstID).
			SetNillableDueAt(tk.DueAt).
			SetLocals(tk.Locals).
			Save(ctx)
		return err
	}
	_, err = c.Token.Create().
		SetID(tk.ID).
		SetInstanceID(tk.InstanceID).
		SetNodeID(tk.NodeID).
		SetStatus(string(tk.Status)).
		SetParentID(tk.ParentID).
		SetChildInstID(tk.ChildInstID).
		SetNillableDueAt(tk.DueAt).
		SetLocals(tk.Locals).
		SetCreatedAt(tk.CreatedAt).
		Save(ctx)
	return err
}

func (t tokenStore) Delete(ctx context.Context, id string) error {
	return t.s.clientOf(ctx).Token.DeleteOneID(id).Exec(ctx)
}

func tokenToFlow(e *gen.Token) *flow.Token {
	return &flow.Token{
		ID:          e.ID,
		InstanceID:  e.InstanceID,
		NodeID:      e.NodeID,
		Status:      flow.TokenStatus(e.Status),
		ParentID:    e.ParentID,
		ChildInstID: e.ChildInstID,
		DueAt:       e.DueAt,
		Locals:      e.Locals,
		CreatedAt:   e.CreatedAt,
	}
}

func (t tokenStore) Get(ctx context.Context, id string) (*flow.Token, error) {
	e, err := t.s.clientOf(ctx).Token.Get(ctx, id)
	if gen.IsNotFound(err) {
		return nil, flow.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return tokenToFlow(e), nil
}

func (t tokenStore) ListByInstance(ctx context.Context, instanceID string) ([]*flow.Token, error) {
	es, err := t.s.clientOf(ctx).Token.Query().Where(token.InstanceIDEQ(instanceID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*flow.Token, 0, len(es))
	for _, e := range es {
		out = append(out, tokenToFlow(e))
	}
	return out, nil
}

func (t tokenStore) ListDueTimers(ctx context.Context, now time.Time) ([]*flow.Token, error) {
	es, err := t.s.clientOf(ctx).Token.Query().
		Where(
			token.StatusEQ(string(flow.TokenWaiting)),
			token.DueAtNotNil(),
			token.DueAtLTE(now),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*flow.Token, 0, len(es))
	for _, e := range es {
		out = append(out, tokenToFlow(e))
	}
	return out, nil
}

// ============================================================
// TaskStore
// ============================================================

type taskStore struct{ s *Store }

func (s *Store) Tasks() flow.TaskStore { return taskStore{s} }

func (t taskStore) upsert(ctx context.Context, x *flow.Task) error {
	c := t.s.clientOf(ctx)
	exists, err := c.Task.Query().Where(task.IDEQ(x.ID)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		_, err = c.Task.UpdateOneID(x.ID).
			SetStatus(string(x.Status)).
			SetOutcome(x.Outcome).
			SetComment(x.Comment).
			SetAssignee(x.Assignee).
			SetNillableDueAt(x.DueAt).
			SetNillableEndedAt(x.EndedAt).
			Save(ctx)
		return err
	}
	_, err = c.Task.Create().
		SetID(x.ID).
		SetInstanceID(x.InstanceID).
		SetTenantID(x.TenantID).
		SetNodeID(x.NodeID).
		SetTokenID(x.TokenID).
		SetName(x.Name).
		SetAssignee(x.Assignee).
		SetStatus(string(x.Status)).
		SetOutcome(x.Outcome).
		SetComment(x.Comment).
		SetNillableDueAt(x.DueAt).
		SetCreatedAt(x.CreatedAt).
		SetNillableEndedAt(x.EndedAt).
		Save(ctx)
	return err
}

func (t taskStore) Save(ctx context.Context, x *flow.Task) error  { return t.upsert(ctx, x) }
func (t taskStore) Update(ctx context.Context, x *flow.Task) error { return t.upsert(ctx, x) }

func taskToFlow(e *gen.Task) *flow.Task {
	return &flow.Task{
		ID:         e.ID,
		InstanceID: e.InstanceID,
		TenantID:   e.TenantID,
		NodeID:     e.NodeID,
		TokenID:    e.TokenID,
		Name:       e.Name,
		Assignee:   e.Assignee,
		Status:     flow.TaskStatus(e.Status),
		Outcome:    e.Outcome,
		Comment:    e.Comment,
		DueAt:      e.DueAt,
		CreatedAt:  e.CreatedAt,
		EndedAt:    e.EndedAt,
	}
}

func (t taskStore) Get(ctx context.Context, id string) (*flow.Task, error) {
	e, err := t.s.clientOf(ctx).Task.Get(ctx, id)
	if gen.IsNotFound(err) {
		return nil, flow.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return taskToFlow(e), nil
}

func tasksToFlow(es []*gen.Task) []*flow.Task {
	out := make([]*flow.Task, 0, len(es))
	for _, e := range es {
		out = append(out, taskToFlow(e))
	}
	return out
}

func (t taskStore) ListByInstance(ctx context.Context, instanceID string) ([]*flow.Task, error) {
	es, err := t.s.clientOf(ctx).Task.Query().
		Where(task.InstanceIDEQ(instanceID)).
		Order(gen.Asc("created_at")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return tasksToFlow(es), nil
}

func (t taskStore) ListByToken(ctx context.Context, tokenID string) ([]*flow.Task, error) {
	es, err := t.s.clientOf(ctx).Task.Query().
		Where(task.TokenIDEQ(tokenID)).
		Order(gen.Asc("created_at")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return tasksToFlow(es), nil
}

func (t taskStore) ListPendingByAssignee(ctx context.Context, tenantID, assignee string) ([]*flow.Task, error) {
	es, err := t.s.clientOf(ctx).Task.Query().
		Where(
			task.StatusEQ(string(flow.TaskPending)),
			task.TenantIDEQ(tenantID),
			task.AssigneeEQ(assignee),
		).
		Order(gen.Asc("created_at")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return tasksToFlow(es), nil
}

func (t taskStore) ListDue(ctx context.Context, now time.Time) ([]*flow.Task, error) {
	es, err := t.s.clientOf(ctx).Task.Query().
		Where(
			task.StatusEQ(string(flow.TaskPending)),
			task.DueAtNotNil(),
			task.DueAtLT(now),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return tasksToFlow(es), nil
}

// ============================================================
// HistoryStore
// ============================================================

type histStore struct{ s *Store }

func (s *Store) History() flow.HistoryStore { return histStore{s} }

func (h histStore) Append(ctx context.Context, ev *flow.HistoryEvent) error {
	_, err := h.s.clientOf(ctx).HistoryEvent.Create().
		SetID(ev.ID).
		SetInstanceID(ev.InstanceID).
		SetType(string(ev.Type)).
		SetNodeID(ev.NodeID).
		SetTaskID(ev.TaskID).
		SetActorID(ev.ActorID).
		SetPayload(ev.Payload).
		SetCreatedAt(ev.CreatedAt).
		Save(ctx)
	return err
}

func (h histStore) ListByInstance(ctx context.Context, instanceID string) ([]*flow.HistoryEvent, error) {
	es, err := h.s.clientOf(ctx).HistoryEvent.Query().
		Where(historyevent.InstanceIDEQ(instanceID)).
		Order(gen.Asc("created_at")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*flow.HistoryEvent, 0, len(es))
	for _, e := range es {
		out = append(out, &flow.HistoryEvent{
			ID:         e.ID,
			InstanceID: e.InstanceID,
			Type:       flow.EventType(e.Type),
			NodeID:     e.NodeID,
			TaskID:     e.TaskID,
			ActorID:    e.ActorID,
			Payload:    e.Payload,
			CreatedAt:  e.CreatedAt,
		})
	}
	return out, nil
}
