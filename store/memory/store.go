// Package memory 提供 flow.Store 的进程内实现：
// 嵌入式用法与单测用；生产环境请实现 ent 版 Store。
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
)

// Store 内存实现。零配置：flow.New(flow.WithStore(memory.New()))。
type Store struct {
	mu     sync.RWMutex
	defs   map[string]*flow.Definition // tenantID|key|version → def
	insts  map[string]*flow.Instance
	tokens map[string]*flow.Token
	tasks  map[string]*flow.Task
	logs   []*flow.HistoryEvent
}

func New() *Store {
	return &Store{
		defs:   map[string]*flow.Definition{},
		insts:  map[string]*flow.Instance{},
		tokens: map[string]*flow.Token{},
		tasks:  map[string]*flow.Task{},
	}
}

// WithTx 事务占位：内存实现无真实事务，fn 直接执行。
// 与 ent/gorm 实现同名同签名，宿主切换存储时无需改动引擎入口包装。
func (s *Store) WithTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

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

// ============================================================
// DefinitionStore
// ============================================================

type defStore struct{ s *Store }

func (s *Store) Definitions() flow.DefinitionStore { return defStore{s} }

func (d defStore) Save(_ context.Context, def *flow.Definition) error {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	cp := def.Clone()
	d.s.defs[defID(def.TenantID, def.Key, def.Version)] = cp
	return nil
}

func (d defStore) Get(_ context.Context, tenantID, key string, version int) (*flow.Definition, error) {
	d.s.mu.RLock()
	defer d.s.mu.RUnlock()
	if version <= 0 {
		// 扫描取最新版本。
		var best *flow.Definition
		for _, def := range d.s.defs {
			if def.TenantID == tenantID && def.Key == key {
				if best == nil || def.Version > best.Version {
					best = def
				}
			}
		}
		if best == nil {
			return nil, flow.ErrNotFound
		}
		return best.Clone(), nil
	}
	def, ok := d.s.defs[defID(tenantID, key, version)]
	if !ok {
		return nil, flow.ErrNotFound
	}
	return def.Clone(), nil
}

func (d defStore) MaxVersion(_ context.Context, tenantID, key string) (int, error) {
	d.s.mu.RLock()
	defer d.s.mu.RUnlock()
	max := 0
	for _, def := range d.s.defs {
		if def.TenantID == tenantID && def.Key == key && def.Version > max {
			max = def.Version
		}
	}
	return max, nil
}

func (d defStore) List(_ context.Context, tenantID, key string) ([]*flow.Definition, error) {
	d.s.mu.RLock()
	defer d.s.mu.RUnlock()
	var out []*flow.Definition
	for _, def := range d.s.defs {
		if def.TenantID == tenantID && (key == "" || def.Key == key) {
			out = append(out, def.Clone())
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Key != out[b].Key {
			return out[a].Key < out[b].Key
		}
		return out[a].Version < out[b].Version
	})
	return out, nil
}

// ============================================================
// InstanceStore
// ============================================================

type instStore struct{ s *Store }

func (s *Store) Instances() flow.InstanceStore { return instStore{s} }

func (i instStore) Save(_ context.Context, inst *flow.Instance) error {
	i.s.mu.Lock()
	defer i.s.mu.Unlock()
	i.s.insts[inst.ID] = cloneInst(inst)
	return nil
}

func (i instStore) Update(_ context.Context, inst *flow.Instance) error {
	i.s.mu.Lock()
	defer i.s.mu.Unlock()
	if _, ok := i.s.insts[inst.ID]; !ok {
		return flow.ErrNotFound
	}
	i.s.insts[inst.ID] = cloneInst(inst)
	return nil
}

func (i instStore) Get(_ context.Context, id string) (*flow.Instance, error) {
	i.s.mu.RLock()
	defer i.s.mu.RUnlock()
	inst, ok := i.s.insts[id]
	if !ok {
		return nil, flow.ErrNotFound
	}
	return cloneInst(inst), nil
}

func (i instStore) ListByStatus(_ context.Context, status flow.InstanceStatus) ([]*flow.Instance, error) {
	i.s.mu.RLock()
	defer i.s.mu.RUnlock()
	var out []*flow.Instance
	for _, inst := range i.s.insts {
		if inst.Status == status {
			out = append(out, cloneInst(inst))
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out, nil
}

func (i instStore) List(_ context.Context, tenantID string) ([]*flow.Instance, error) {
	i.s.mu.RLock()
	defer i.s.mu.RUnlock()
	var out []*flow.Instance
	for _, inst := range i.s.insts {
		if inst.TenantID == tenantID {
			out = append(out, cloneInst(inst))
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out, nil
}

func cloneInst(in *flow.Instance) *flow.Instance {
	cp := *in
	cp.Variables = in.Variables.Clone()
	return &cp
}

// ============================================================
// TokenStore
// ============================================================

type tokenStore struct{ s *Store }

func (s *Store) Tokens() flow.TokenStore { return tokenStore{s} }

func (t tokenStore) Save(_ context.Context, tk *flow.Token) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.s.tokens[tk.ID] = cloneToken(tk)
	return nil
}

func (t tokenStore) Update(_ context.Context, tk *flow.Token) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if _, ok := t.s.tokens[tk.ID]; !ok {
		return flow.ErrNotFound
	}
	t.s.tokens[tk.ID] = cloneToken(tk)
	return nil
}

func (t tokenStore) Delete(_ context.Context, id string) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	delete(t.s.tokens, id)
	return nil
}

func (t tokenStore) Get(_ context.Context, id string) (*flow.Token, error) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	tk, ok := t.s.tokens[id]
	if !ok {
		return nil, flow.ErrNotFound
	}
	return cloneToken(tk), nil
}

func (t tokenStore) ListByInstance(_ context.Context, instanceID string) ([]*flow.Token, error) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	var out []*flow.Token
	for _, tk := range t.s.tokens {
		if tk.InstanceID == instanceID {
			out = append(out, cloneToken(tk))
		}
	}
	return out, nil
}

func (t tokenStore) ListDueTimers(_ context.Context, now time.Time) ([]*flow.Token, error) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	var out []*flow.Token
	for _, tk := range t.s.tokens {
		if tk.Status == flow.TokenWaiting && tk.DueAt != nil && !tk.DueAt.After(now) {
			out = append(out, cloneToken(tk))
		}
	}
	return out, nil
}

func cloneToken(in *flow.Token) *flow.Token {
	cp := *in
	return &cp
}

// ============================================================
// TaskStore
// ============================================================

type taskStore struct{ s *Store }

func (s *Store) Tasks() flow.TaskStore { return taskStore{s} }

func (t taskStore) Save(_ context.Context, task *flow.Task) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.s.tasks[task.ID] = cloneTask(task)
	return nil
}

func (t taskStore) Update(_ context.Context, task *flow.Task) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if _, ok := t.s.tasks[task.ID]; !ok {
		return flow.ErrNotFound
	}
	t.s.tasks[task.ID] = cloneTask(task)
	return nil
}

func (t taskStore) Get(_ context.Context, id string) (*flow.Task, error) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	task, ok := t.s.tasks[id]
	if !ok {
		return nil, flow.ErrNotFound
	}
	return cloneTask(task), nil
}

func (t taskStore) ListByInstance(_ context.Context, instanceID string) ([]*flow.Task, error) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	var out []*flow.Task
	for _, task := range t.s.tasks {
		if task.InstanceID == instanceID {
			out = append(out, cloneTask(task))
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out, nil
}

func (t taskStore) ListByToken(_ context.Context, tokenID string) ([]*flow.Task, error) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	var out []*flow.Task
	for _, task := range t.s.tasks {
		if task.TokenID == tokenID {
			out = append(out, cloneTask(task))
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out, nil
}

func (t taskStore) ListPendingByAssignee(_ context.Context, tenantID, assignee string) ([]*flow.Task, error) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	var out []*flow.Task
	for _, task := range t.s.tasks {
		if task.Status == flow.TaskPending && task.TenantID == tenantID && task.Assignee == assignee {
			out = append(out, cloneTask(task))
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out, nil
}

func (t taskStore) ListDue(_ context.Context, now time.Time) ([]*flow.Task, error) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	var out []*flow.Task
	for _, task := range t.s.tasks {
		if task.Status == flow.TaskPending && task.DueAt != nil && task.DueAt.Before(now) {
			out = append(out, cloneTask(task))
		}
	}
	return out, nil
}

func cloneTask(in *flow.Task) *flow.Task {
	cp := *in
	return &cp
}

// ============================================================
// HistoryStore
// ============================================================

type histStore struct{ s *Store }

func (s *Store) History() flow.HistoryStore { return histStore{s} }

func (h histStore) Append(_ context.Context, ev *flow.HistoryEvent) error {
	h.s.mu.Lock()
	defer h.s.mu.Unlock()
	cp := *ev
	h.s.logs = append(h.s.logs, &cp)
	return nil
}

func (h histStore) ListByInstance(_ context.Context, instanceID string) ([]*flow.HistoryEvent, error) {
	h.s.mu.RLock()
	defer h.s.mu.RUnlock()
	var out []*flow.HistoryEvent
	for _, ev := range h.s.logs {
		if ev.InstanceID == instanceID {
			cp := *ev
			out = append(out, &cp)
		}
	}
	return out, nil
}
