// Package gormstore 提供 flow.Store 的 GORM 实现。
//
// 事务模型与 ent 实现（store/ent）一致：宿主在引擎入口用 WithTx 包裹，
// 一次引擎调用内的全部落库共享一个事务；ctx 中已有事务时直接复用。
//
// 数据库兼容：纯 CRUD + JSON 序列化字段，MySQL / PostgreSQL / SQLite 等主流
// 方言均可（AutoMigrate 建表；生产建议接入宿主的版本化迁移）。
package gormstore

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tx7do/go-wind-flow/flow"
)

// Store GORM 实现。构造：gormstore.New(db)；建表用 AutoMigrate 或宿主迁移。
type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store { return &Store{db: db} }

// AutoMigrate 建表便捷方法（生产建议用版本化迁移替代）。
func (s *Store) AutoMigrate() error {
	return s.db.AutoMigrate(
		&Definition{}, &Instance{}, &Token{}, &Task{}, &HistoryEvent{},
	)
}

// ============================================================
// 事务
// ============================================================

type txKey struct{}

func (s *Store) WithTx(ctx context.Context, fn func(context.Context) error) error {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return fn(ctx) // 已在外层事务内，复用
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// dbOf 返回当前 ctx 绑定的执行句柄（事务内返回事务句柄）。
func (s *Store) dbOf(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return s.db.WithContext(ctx)
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
	m := &Definition{
		ID:        defID(def.TenantID, def.Key, def.Version),
		TenantID:  def.TenantID,
		DefKey:    def.Key,
		Version:   def.Version,
		Name:      def.Name,
		Format:    def.Format,
		Source:    string(def.Source),
		Nodes:     def.Nodes,
		Edges:     def.Edges,
		CreatedAt: def.CreatedAt,
	}
	// upsert：复合键冲突时整行替换（幂等 Save）。
	return d.s.dbOf(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(m).Error
}

func defToFlow(m *Definition) *flow.Definition {
	return &flow.Definition{
		Key:       m.DefKey,
		Name:      m.Name,
		Version:   m.Version,
		TenantID:  m.TenantID,
		Format:    m.Format,
		Source:    []byte(m.Source),
		CreatedAt: m.CreatedAt,
		Nodes:     m.Nodes,
		Edges:     m.Edges,
	}
}

func (d defStore) Get(ctx context.Context, tenantID, key string, version int) (*flow.Definition, error) {
	q := d.s.dbOf(ctx).Where("tenant_id = ? AND def_key = ?", tenantID, key)
	if version > 0 {
		q = q.Where("version = ?", version)
	}
	var m Definition
	if err := q.Order("version DESC").First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, flow.ErrNotFound
		}
		return nil, err
	}
	return defToFlow(&m), nil
}

func (d defStore) MaxVersion(ctx context.Context, tenantID, key string) (int, error) {
	var m Definition
	err := d.s.dbOf(ctx).
		Where("tenant_id = ? AND def_key = ?", tenantID, key).
		Order("version DESC").First(&m).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return m.Version, nil
}

func (d defStore) List(ctx context.Context, tenantID, key string) ([]*flow.Definition, error) {
	q := d.s.dbOf(ctx).Where("tenant_id = ?", tenantID)
	if key != "" {
		q = q.Where("def_key = ?", key)
	}
	var ms []Definition
	if err := q.Order("def_key ASC, version ASC").Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*flow.Definition, 0, len(ms))
	for idx := range ms {
		out = append(out, defToFlow(&ms[idx]))
	}
	return out, nil
}

// ============================================================
// InstanceStore
// ============================================================

type instStore struct{ s *Store }

func (s *Store) Instances() flow.InstanceStore { return instStore{s} }

func instToModel(inst *flow.Instance) *Instance {
	return &Instance{
		ID:                inst.ID,
		TenantID:          inst.TenantID,
		DefinitionKey:     inst.DefinitionKey,
		DefinitionVersion: inst.DefinitionVersion,
		BusinessKey:       inst.BusinessKey,
		BusinessType:      inst.BusinessType,
		StarterID:         inst.StarterID,
		Status:            string(inst.Status),
		Variables:         inst.Variables,
		ParentInstanceID:  inst.ParentInstanceID,
		ParentTokenID:     inst.ParentTokenID,
		CreatedAt:         inst.CreatedAt,
		EndedAt:           inst.EndedAt,
	}
}

func instToFlow(m *Instance) *flow.Instance {
	return &flow.Instance{
		ID:                m.ID,
		TenantID:          m.TenantID,
		DefinitionKey:     m.DefinitionKey,
		DefinitionVersion: m.DefinitionVersion,
		BusinessKey:       m.BusinessKey,
		BusinessType:      m.BusinessType,
		StarterID:         m.StarterID,
		Status:            flow.InstanceStatus(m.Status),
		Variables:         m.Variables,
		ParentInstanceID:  m.ParentInstanceID,
		ParentTokenID:     m.ParentTokenID,
		CreatedAt:         m.CreatedAt,
		EndedAt:           m.EndedAt,
	}
}

func (i instStore) upsert(ctx context.Context, inst *flow.Instance) error {
	if err := i.s.dbOf(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(instToModel(inst)).Error; err != nil {
		return err
	}
	return nil
}

func (i instStore) Save(ctx context.Context, inst *flow.Instance) error   { return i.upsert(ctx, inst) }
func (i instStore) Update(ctx context.Context, inst *flow.Instance) error { return i.upsert(ctx, inst) }

func (i instStore) Get(ctx context.Context, id string) (*flow.Instance, error) {
	var m Instance
	if err := i.s.dbOf(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, flow.ErrNotFound
		}
		return nil, err
	}
	return instToFlow(&m), nil
}

func (i instStore) ListByStatus(ctx context.Context, status flow.InstanceStatus) ([]*flow.Instance, error) {
	var ms []Instance
	if err := i.s.dbOf(ctx).
		Where("status = ?", string(status)).
		Order("created_at ASC").
		Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*flow.Instance, 0, len(ms))
	for idx := range ms {
		out = append(out, instToFlow(&ms[idx]))
	}
	return out, nil
}

func (i instStore) List(ctx context.Context, tenantID string) ([]*flow.Instance, error) {
	var ms []Instance
	if err := i.s.dbOf(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at ASC").
		Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*flow.Instance, 0, len(ms))
	for idx := range ms {
		out = append(out, instToFlow(&ms[idx]))
	}
	return out, nil
}

// ============================================================
// TokenStore
// ============================================================

type tokenStore struct{ s *Store }

func (s *Store) Tokens() flow.TokenStore { return tokenStore{s} }

func tokenToModel(tk *flow.Token) *Token {
	return &Token{
		ID:          tk.ID,
		InstanceID:  tk.InstanceID,
		NodeID:      tk.NodeID,
		Status:      string(tk.Status),
		ParentID:    tk.ParentID,
		ChildInstID: tk.ChildInstID,
		DueAt:       tk.DueAt,
		Locals:      tk.Locals,
		CreatedAt:   tk.CreatedAt,
	}
}

func tokenToFlow(m *Token) *flow.Token {
	return &flow.Token{
		ID:          m.ID,
		InstanceID:  m.InstanceID,
		NodeID:      m.NodeID,
		Status:      flow.TokenStatus(m.Status),
		ParentID:    m.ParentID,
		ChildInstID: m.ChildInstID,
		DueAt:       m.DueAt,
		Locals:      m.Locals,
		CreatedAt:   m.CreatedAt,
	}
}

func (t tokenStore) upsert(ctx context.Context, tk *flow.Token) error {
	return t.s.dbOf(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(tokenToModel(tk)).Error
}

func (t tokenStore) Save(ctx context.Context, tk *flow.Token) error   { return t.upsert(ctx, tk) }
func (t tokenStore) Update(ctx context.Context, tk *flow.Token) error { return t.upsert(ctx, tk) }

func (t tokenStore) Delete(ctx context.Context, id string) error {
	return t.s.dbOf(ctx).Where("id = ?", id).Delete(&Token{}).Error
}

func (t tokenStore) Get(ctx context.Context, id string) (*flow.Token, error) {
	var m Token
	if err := t.s.dbOf(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, flow.ErrNotFound
		}
		return nil, err
	}
	return tokenToFlow(&m), nil
}

func (t tokenStore) ListByInstance(ctx context.Context, instanceID string) ([]*flow.Token, error) {
	var ms []Token
	if err := t.s.dbOf(ctx).Where("instance_id = ?", instanceID).Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*flow.Token, 0, len(ms))
	for idx := range ms {
		out = append(out, tokenToFlow(&ms[idx]))
	}
	return out, nil
}

func (t tokenStore) ListDueTimers(ctx context.Context, now time.Time) ([]*flow.Token, error) {
	var ms []Token
	if err := t.s.dbOf(ctx).
		Where("status = ? AND due_at IS NOT NULL AND due_at <= ?",
			string(flow.TokenWaiting), now).
		Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*flow.Token, 0, len(ms))
	for idx := range ms {
		out = append(out, tokenToFlow(&ms[idx]))
	}
	return out, nil
}

// ============================================================
// TaskStore
// ============================================================

type taskStore struct{ s *Store }

func (s *Store) Tasks() flow.TaskStore { return taskStore{s} }

func taskToModel(x *flow.Task) *Task {
	return &Task{
		ID:              x.ID,
		InstanceID:      x.InstanceID,
		TenantID:        x.TenantID,
		NodeID:          x.NodeID,
		TokenID:         x.TokenID,
		Name:            x.Name,
		Assignee:        x.Assignee,
		CandidateGroups: x.CandidateGroups,
		Status:          string(x.Status),
		Outcome:         x.Outcome,
		Comment:         x.Comment,
		DueAt:           x.DueAt,
		CreatedAt:       x.CreatedAt,
		EndedAt:         x.EndedAt,
	}
}

func taskToFlow(m *Task) *flow.Task {
	return &flow.Task{
		ID:              m.ID,
		InstanceID:      m.InstanceID,
		TenantID:        m.TenantID,
		NodeID:          m.NodeID,
		TokenID:         m.TokenID,
		Name:            m.Name,
		Assignee:        m.Assignee,
		CandidateGroups: m.CandidateGroups,
		Status:          flow.TaskStatus(m.Status),
		Outcome:         m.Outcome,
		Comment:         m.Comment,
		DueAt:           m.DueAt,
		CreatedAt:       m.CreatedAt,
		EndedAt:         m.EndedAt,
	}
}

func (t taskStore) upsert(ctx context.Context, x *flow.Task) error {
	return t.s.dbOf(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(taskToModel(x)).Error
}

func (t taskStore) Save(ctx context.Context, x *flow.Task) error   { return t.upsert(ctx, x) }
func (t taskStore) Update(ctx context.Context, x *flow.Task) error { return t.upsert(ctx, x) }

func (t taskStore) Get(ctx context.Context, id string) (*flow.Task, error) {
	var m Task
	if err := t.s.dbOf(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, flow.ErrNotFound
		}
		return nil, err
	}
	return taskToFlow(&m), nil
}

func (t taskStore) tasksQuery(ctx context.Context, query string, args ...any) ([]*flow.Task, error) {
	var ms []Task
	if err := t.s.dbOf(ctx).Where(query, args...).Order("created_at ASC").Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*flow.Task, 0, len(ms))
	for idx := range ms {
		out = append(out, taskToFlow(&ms[idx]))
	}
	return out, nil
}

func (t taskStore) ListByInstance(ctx context.Context, instanceID string) ([]*flow.Task, error) {
	return t.tasksQuery(ctx, "instance_id = ?", instanceID)
}

func (t taskStore) ListByToken(ctx context.Context, tokenID string) ([]*flow.Task, error) {
	return t.tasksQuery(ctx, "token_id = ?", tokenID)
}

func (t taskStore) ListPendingByAssignee(ctx context.Context, tenantID, assignee string) ([]*flow.Task, error) {
	return t.tasksQuery(ctx,
		"status = ? AND tenant_id = ? AND assignee = ?",
		string(flow.TaskPending), tenantID, assignee)
}

func (t taskStore) ListUnclaimedByGroups(ctx context.Context, tenantID string, groups []string) ([]*flow.Task, error) {
	// JSON 数组字段跨方言 contains不可移植——查未分派待办后内存过滤。
	var ms []Task
	if err := t.s.dbOf(ctx).
		Where("status = ? AND assignee = '' AND tenant_id = ?",
			string(flow.TaskPending), tenantID).
		Order("created_at ASC").
		Find(&ms).Error; err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, g := range groups {
		want[g] = true
	}
	out := make([]*flow.Task, 0, len(ms))
	for idx := range ms {
		for _, g := range ms[idx].CandidateGroups {
			if want[g] {
				out = append(out, taskToFlow(&ms[idx]))
				break
			}
		}
	}
	return out, nil
}

func (t taskStore) ListDue(ctx context.Context, now time.Time) ([]*flow.Task, error) {
	var ms []Task
	if err := t.s.dbOf(ctx).
		Where("status = ? AND due_at IS NOT NULL AND due_at < ?",
			string(flow.TaskPending), now).
		Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*flow.Task, 0, len(ms))
	for idx := range ms {
		out = append(out, taskToFlow(&ms[idx]))
	}
	return out, nil
}

// ============================================================
// HistoryStore
// ============================================================

type histStore struct{ s *Store }

func (s *Store) History() flow.HistoryStore { return histStore{s} }

func (h histStore) Append(ctx context.Context, ev *flow.HistoryEvent) error {
	m := &HistoryEvent{
		ID:         ev.ID,
		InstanceID: ev.InstanceID,
		Type:       string(ev.Type),
		NodeID:     ev.NodeID,
		TaskID:     ev.TaskID,
		ActorID:    ev.ActorID,
		Payload:    ev.Payload,
		CreatedAt:  ev.CreatedAt,
	}
	if err := h.s.dbOf(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("flow/gormstore: append history: %w", err)
	}
	return nil
}

func (h histStore) ListByInstance(ctx context.Context, instanceID string) ([]*flow.HistoryEvent, error) {
	var ms []HistoryEvent
	if err := h.s.dbOf(ctx).
		Where("instance_id = ?", instanceID).
		Order("created_at ASC").
		Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*flow.HistoryEvent, 0, len(ms))
	for idx := range ms {
		m := &ms[idx]
		out = append(out, &flow.HistoryEvent{
			ID:         m.ID,
			InstanceID: m.InstanceID,
			Type:       flow.EventType(m.Type),
			NodeID:     m.NodeID,
			TaskID:     m.TaskID,
			ActorID:    m.ActorID,
			Payload:    m.Payload,
			CreatedAt:  m.CreatedAt,
		})
	}
	return out, nil
}
