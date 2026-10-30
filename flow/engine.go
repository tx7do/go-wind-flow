package flow

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Engine 流程引擎。
//
// 并发约定：引擎自身无锁。同一实例的引擎入口调用（Start/CompleteTask/
// Withdraw/Abort）必须由宿主串行化——进程内按 instanceID 加锁，或由
// 存储层事务/行级锁保证。子流程经由父实例调用链天然串行。
type Engine struct {
	store     Store
	evaluator ExpressionEvaluator
	scripts   ScriptRunner
	identity  IdentityProvider
	clock     Clock
	idgen     IDGen
	log       Logger
	importers *FormatRegistry
	exporters *FormatRegistry
	listeners []Listener
	maxSteps  int
}

type Option func(*Engine)

func WithStore(s Store) Option                   { return func(e *Engine) { e.store = s } }
func WithEvaluator(v ExpressionEvaluator) Option { return func(e *Engine) { e.evaluator = v } }
func WithScripts(s ScriptRunner) Option          { return func(e *Engine) { e.scripts = s } }
func WithIdentity(p IdentityProvider) Option     { return func(e *Engine) { e.identity = p } }
func WithClock(c Clock) Option                   { return func(e *Engine) { e.clock = c } }
func WithIDGen(g IDGen) Option                   { return func(e *Engine) { e.idgen = g } }
func WithLogger(l Logger) Option                 { return func(e *Engine) { e.log = l } }
func WithMaxSteps(n int) Option                  { return func(e *Engine) { e.maxSteps = n } }

// WithImporters 注册定义导入器（bpmn.Importer、windjson.Importer 等）。
func WithImporters(im ...Importer) Option {
	return func(e *Engine) {
		for _, i := range im {
			e.importers.RegisterImporter(i)
		}
	}
}

// WithExporters 注册定义导出器。
func WithExporters(ex ...Exporter) Option {
	return func(e *Engine) {
		for _, x := range ex {
			e.exporters.RegisterExporter(x)
		}
	}
}

// WithListener 注册全局监听器（按注册顺序调用）。
func WithListener(l ...Listener) Option {
	return func(e *Engine) { e.listeners = append(e.listeners, l...) }
}

func New(opts ...Option) *Engine {
	e := &Engine{
		log:       nopLogger{},
		clock:     realClock{},
		idgen:     defaultIDGen,
		maxSteps:  10000,
		importers: NewFormatRegistry(),
		exporters: NewFormatRegistry(),
		evaluator: NewSimpleEvaluator(),
	}
	// 内容嗅探：'<' 开头视为 BPMN XML，否则按 wind-json-v2（与 go-wind-oa/im 兼容）。
	e.importers.SetDetector(func(data []byte) string {
		s := strings.TrimSpace(string(data))
		if strings.HasPrefix(s, "<") {
			return "bpmn-xml"
		}
		return "wind-json-v2"
	})
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ============================================================
// 定义管理
// ============================================================

type DeployRequest struct {
	TenantID string
	Key      string
	Name     string
	// Format 为空时按内容嗅探（'<' → bpmn-xml，否则 wind-json-v2）。
	Format string
	Data   []byte
}

// Deploy 部署（或升版）一份流程定义。定义不可变：每次 Deploy 生成新版本。
func (e *Engine) Deploy(ctx context.Context, req DeployRequest) (*Definition, error) {
	if req.Key == "" {
		return nil, fmt.Errorf("flow: deploy requires key")
	}
	im, err := e.importers.ResolveImport(req.Format, req.Data)
	if err != nil {
		return nil, err
	}
	def, err := im.Import(req.Data)
	if err != nil {
		return nil, fmt.Errorf("import definition %s: %w", req.Key, err)
	}
	def.Key = req.Key
	if req.Name != "" {
		def.Name = req.Name
	}
	def.TenantID = req.TenantID
	def.Format = im.Format()
	def.Source = append([]byte(nil), req.Data...)
	def.CreatedAt = e.clock.Now()

	if err := ValidateDefinition(def); err != nil {
		return nil, err
	}

	maxV, err := e.store.Definitions().MaxVersion(ctx, req.TenantID, req.Key)
	if err != nil {
		return nil, err
	}
	def.Version = maxV + 1

	if err := e.store.Definitions().Save(ctx, def); err != nil {
		return nil, err
	}
	e.log.Printf("flow: deployed definition %s v%d (%s, %d nodes/%d edges)",
		def.Key, def.Version, def.Format, len(def.Nodes), len(def.Edges))
	return def, nil
}

// GetDefinition 查询定义；version<=0 取最新。
func (e *Engine) GetDefinition(ctx context.Context, tenantID, key string, version int) (*Definition, error) {
	return e.store.Definitions().Get(ctx, tenantID, key, version)
}

// ExportDefinition 以指定格式导出定义。
func (e *Engine) ExportDefinition(ctx context.Context, tenantID, key string, version int, format string) ([]byte, error) {
	def, err := e.GetDefinition(ctx, tenantID, key, version)
	if err != nil {
		return nil, err
	}
	e.exporters.mu.RLock()
	ex, ok := e.exporters.exporters[format]
	e.exporters.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no exporter registered for format %q", format)
	}
	return ex.Export(def)
}

// ============================================================
// 实例生命周期
// ============================================================

type StartRequest struct {
	TenantID     string
	Key          string
	Version      int // 0 = latest
	BusinessKey  string
	BusinessType string
	StarterID    string
	Variables    Vars
	// ParentInstanceID / ParentTokenID 子流程链接：由引擎在 callActivity
	// 处自动填充；宿主直接调用时不应设置。
	ParentInstanceID string
	ParentTokenID    string
}

// Start 启动流程实例：创建实例 + 根令牌，同步推进到首个阻塞点。
// 返回实例与首个阻塞处创建的用户任务 ID 列表（可能为空）。
func (e *Engine) Start(ctx context.Context, req StartRequest) (*Instance, []string, error) {
	def, err := e.store.Definitions().Get(ctx, req.TenantID, req.Key, req.Version)
	if err != nil {
		return nil, nil, err
	}
	if def.Version == 0 && req.Version > 0 {
		return nil, nil, fmt.Errorf("%w: definition %s v%d", ErrNotFound, req.Key, req.Version)
	}

	now := e.clock.Now()
	inst := &Instance{
		ID:                e.idgen(),
		TenantID:          req.TenantID,
		DefinitionKey:     def.Key,
		DefinitionVersion: def.Version,
		BusinessKey:       req.BusinessKey,
		BusinessType:      req.BusinessType,
		StarterID:         req.StarterID,
		Status:            InstanceRunning,
		Variables:         req.Variables.Clone(),
		ParentInstanceID:  req.ParentInstanceID,
		ParentTokenID:     req.ParentTokenID,
		CreatedAt:         now,
	}
	if inst.Variables == nil {
		inst.Variables = Vars{}
	}
	if err := e.store.Instances().Save(ctx, inst); err != nil {
		return nil, nil, err
	}
	_ = e.appendHistory(ctx, inst, &Event{Type: EventProcessStarted, ActorID: req.StarterID}, Vars{"businessKey": req.BusinessKey})

	start := findStart(def)
	if start == "" {
		_ = e.abortInstance(ctx, inst, "definition has no start event")
		return nil, nil, fmt.Errorf("%w: no start event", ErrInvalidDefinition)
	}
	root := &Token{
		ID:         e.idgen(),
		InstanceID: inst.ID,
		NodeID:     start,
		Status:     TokenRunning,
		CreatedAt:  now,
	}
	if err := e.store.Tokens().Save(ctx, root); err != nil {
		return nil, nil, err
	}

	tasksBefore, _ := e.store.Tasks().ListByInstance(ctx, inst.ID)
	if err := e.runToken(ctx, inst, root, def); err != nil {
		return inst, nil, err
	}
	tasksAfter, _ := e.store.Tasks().ListByInstance(ctx, inst.ID)

	created := taskIDs(diff(tasksAfter, tasksBefore))
	fresh, _ := e.store.Instances().Get(ctx, inst.ID)
	if fresh != nil {
		inst = fresh
	}
	return inst, created, nil
}

type CompleteTaskRequest struct {
	TaskID  string
	ActorID string
	// Outcome: OutcomeApprove / OutcomeReject
	Outcome string
	Comment string
	// Variables 办结时回写的业务变量（如表单字段），进实例级。
	Variables Vars
	// LocalVariables 办结时写入令牌级局部变量（仅当前分支后续节点可读）。
	LocalVariables Vars
}

// CompleteTask 办结用户任务。会签/或签语义由定义驱动；驳回按 RejectMode 处理。
func (e *Engine) CompleteTask(ctx context.Context, req CompleteTaskRequest) error {
	task, err := e.store.Tasks().Get(ctx, req.TaskID)
	if err != nil {
		return err
	}
	if task.Status != TaskPending {
		return fmt.Errorf("%w: task %s already %s", ErrInvalidState, task.ID, task.Status)
	}
	if req.ActorID == "" || req.ActorID != task.Assignee {
		return fmt.Errorf("%w: actor %q is not assignee of task %s", ErrForbidden, req.ActorID, task.ID)
	}

	inst, err := e.store.Instances().Get(ctx, task.InstanceID)
	if err != nil {
		return err
	}
	if !inst.Status.Live() {
		return fmt.Errorf("%w: instance %s is %s", ErrInvalidState, inst.ID, inst.Status)
	}

	outcome := strings.ToUpper(strings.TrimSpace(req.Outcome))
	switch outcome {
	case OutcomeApprove, OutcomeReject:
	default:
		return fmt.Errorf("%w: unknown outcome %q", ErrInvalidState, req.Outcome)
	}

	now := e.clock.Now()
	task.Status = mapOutcome(outcome)
	task.Outcome = outcome
	task.Comment = req.Comment
	task.EndedAt = &now
	if err := e.store.Tasks().Update(ctx, task); err != nil {
		return err
	}
	_ = e.appendHistory(ctx, inst, &Event{
		Type: EventTaskCompleted, NodeID: task.NodeID, TaskID: task.ID, ActorID: req.ActorID,
	}, Vars{"outcome": outcome, "comment": req.Comment})

	if len(req.Variables) > 0 {
		inst.Variables.Merge(req.Variables)
		if err := e.store.Instances().Update(ctx, inst); err != nil {
			return err
		}
		_ = e.appendHistory(ctx, inst, &Event{Type: EventVariableChanged}, req.Variables)
	}

	tk, err := e.store.Tokens().Get(ctx, task.TokenID)
	if err != nil {
		return err
	}
	if len(req.LocalVariables) > 0 {
		if tk.Locals == nil {
			tk.Locals = Vars{}
		}
		tk.Locals.Merge(req.LocalVariables)
		if err := e.store.Tokens().Update(ctx, tk); err != nil {
			return err
		}
	}
	def, err := e.definitionOf(ctx, inst)
	if err != nil {
		return err
	}

	if outcome == OutcomeReject {
		node := def.Nodes[task.NodeID]
		if node != nil && node.Approval.RejectMode == RejectStay {
			// 停留模式：任务已标 REJECTED，实例继续（由宿主监听器决定补救路径）。
			return nil
		}
		return e.rejectInstance(ctx, inst, req.ActorID)
	}

	// 会签（ALL）：还有未办结的同令牌任务则继续等待。
	siblings, err := e.store.Tasks().ListByToken(ctx, task.TokenID)
	if err != nil {
		return err
	}
	strategy := StrategyAll
	if node := def.Nodes[task.NodeID]; node != nil && node.Approval.Strategy == StrategyAny {
		strategy = StrategyAny
	}
	if strategy == StrategyAll {
		for _, sib := range siblings {
			if sib.Status == TaskPending {
				return nil // 仍在等待其他办理人
			}
		}
	} else {
		// 或签：任一通过即取消同令牌其余待办。
		for _, sib := range siblings {
			if sib.Status == TaskPending {
				sib.Status = TaskCancelled
				end := e.clock.Now()
				sib.EndedAt = &end
				if err := e.store.Tasks().Update(ctx, sib); err != nil {
					return err
				}
			}
		}
	}

	// 全员通过 / 或签命中：令牌沿唯一出边继续推进（离开任务节点）。
	tk.Status = TokenRunning
	if out := def.OutEdges(task.NodeID); len(out) == 1 {
		tk.NodeID = out[0].To
	}
	if err := e.store.Tokens().Update(ctx, tk); err != nil {
		return err
	}
	return e.runToken(ctx, inst, tk, def)
}

// AddTaskApprovers 加签：向待办任务所在节点动态追加办理人（会签式——
// 新任务与既有待办并行，ALL/ANY 语义照常）。
//
// 委托叠加与任务创建时的解析一致；已是该节点待办办理人的用户自动去重。
// 返回本次新创建的任务。
func (e *Engine) AddTaskApprovers(ctx context.Context, taskID, actorID string, userIDs []string) ([]*Task, error) {
	task, err := e.store.Tasks().Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != TaskPending {
		return nil, fmt.Errorf("%w: task %s not pending", ErrInvalidState, task.ID)
	}
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("flow: empty approvers")
	}
	inst, err := e.store.Instances().Get(ctx, task.InstanceID)
	if err != nil {
		return nil, err
	}
	if !inst.Status.Live() {
		return nil, fmt.Errorf("%w: instance %s is %s", ErrInvalidState, inst.ID, inst.Status)
	}
	def, err := e.definitionOf(ctx, inst)
	if err != nil {
		return nil, err
	}
	node := def.Nodes[task.NodeID]

	// 委托叠加（与 arriveAtApprovers 创建语义一致）。
	if e.identity != nil {
		for i, uid := range userIDs {
			delegate, derr := e.identity.ResolveDelegate(ctx, inst.TenantID, uid)
			if derr != nil {
				e.log.Printf("flow: resolve delegation for %s failed: %v (keep original)", uid, derr)
				continue
			}
			userIDs[i] = delegate
		}
	}

	// 既有待办办理人去重。
	siblings, err := e.store.Tasks().ListByToken(ctx, task.TokenID)
	if err != nil {
		return nil, err
	}
	existing := map[string]bool{}
	for _, sib := range siblings {
		if sib.Status == TaskPending {
			existing[sib.Assignee] = true
		}
	}

	now := e.clock.Now()
	tk, err := e.store.Tokens().Get(ctx, task.TokenID)
	if err != nil {
		return nil, err
	}
	tk.Status = TokenWaiting // 令牌本就 WAITING，显式保持
	if err := e.store.Tokens().Update(ctx, tk); err != nil {
		return nil, err
	}

	var created []*Task
	for _, uid := range userIDs {
		if uid == "" || existing[uid] {
			continue
		}
		existing[uid] = true
		nt := &Task{
			ID:         e.idgen(),
			InstanceID: inst.ID,
			TenantID:   inst.TenantID,
			NodeID:     task.NodeID,
			TokenID:    task.TokenID,
			Name:       task.Name,
			Assignee:   uid,
			Status:     TaskPending,
			CreatedAt:  now,
		}
		if node != nil && node.Timer != nil {
			due := now.Add(node.Timer.Duration.Std())
			nt.DueAt = &due
		}
		if err := e.store.Tasks().Save(ctx, nt); err != nil {
			return nil, err
		}
		created = append(created, nt)
		_ = e.appendHistory(ctx, inst, &Event{
			Type: EventTaskApproverAdded, NodeID: task.NodeID, TaskID: nt.ID, ActorID: actorID,
		}, Vars{"assignee": uid, "anchorTask": task.ID})
		e.fireListeners(ctx, &Event{
			Type: EventTaskApproverAdded, TenantID: inst.TenantID, InstanceID: inst.ID,
			TaskID: nt.ID, NodeID: task.NodeID, ActorID: actorID,
			Vars: inst.Variables.Clone(), At: now,
		})
	}
	return created, nil
}

// ReturnTo 驳回指定节点：实例退回到更早的 userTask 重新走（如退回发起人重新提交）。
//
// 约束（fail-closed）：
//   - 目标必须是当前节点的 userTask 祖先（当前节点自目标可达）；
//   - 实例在途并行分支多于一条时不支持（避免 fork/汇合态回滚的语义难题）。
//
// 语义：取消全部待办与在途令牌，实例保持 RUNNING，自目标节点生成新令牌
// 重走——动态办理人规则届时重新解析。历史完整保留。
func (e *Engine) ReturnTo(ctx context.Context, taskID, actorID, targetNodeID, comment string) error {
	task, err := e.store.Tasks().Get(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status != TaskPending {
		return fmt.Errorf("%w: task %s not pending", ErrInvalidState, task.ID)
	}
	inst, err := e.store.Instances().Get(ctx, task.InstanceID)
	if err != nil {
		return err
	}
	if !inst.Status.Live() {
		return fmt.Errorf("%w: instance %s is %s", ErrInvalidState, inst.ID, inst.Status)
	}
	def, err := e.definitionOf(ctx, inst)
	if err != nil {
		return err
	}
	target := def.Nodes[targetNodeID]
	if target == nil {
		return fmt.Errorf("%w: return-to target %q not in definition", ErrInvalidState, targetNodeID)
	}
	if target.Type != NodeUserTask {
		return fmt.Errorf("%w: return-to target %s is %s, want userTask", ErrInvalidState, targetNodeID, target.Type)
	}
	if targetNodeID == task.NodeID || !reachableFrom(def, targetNodeID, task.NodeID) {
		return fmt.Errorf("%w: node %s is not an ancestor of %s", ErrInvalidState, targetNodeID, task.NodeID)
	}

	// 在途并行分支 fail-closed。
	tokens, err := e.store.Tokens().ListByInstance(ctx, inst.ID)
	if err != nil {
		return err
	}
	live := 0
	for _, tk := range tokens {
		if tk.Status != TokenEnded {
			live++
		}
	}
	if live > 1 {
		return fmt.Errorf("%w: return-to unsupported with %d live tokens (parallel in flight)", ErrInvalidState, live)
	}

	// 取消全部待办、终结全部令牌。
	tasks, _ := e.store.Tasks().ListByInstance(ctx, inst.ID)
	for _, t := range tasks {
		if t.Status == TaskPending {
			now := e.clock.Now()
			t.Status = TaskCancelled
			t.EndedAt = &now
			_ = e.store.Tasks().Update(ctx, t)
		}
	}
	for _, tk := range tokens {
		if tk.Status != TokenEnded {
			tk.Status = TokenEnded
			_ = e.store.Tokens().Update(ctx, tk)
		}
	}

	_ = e.appendHistory(ctx, inst, &Event{
		Type: EventInstanceReturned, NodeID: targetNodeID, ActorID: actorID,
	}, Vars{"from": task.NodeID, "to": targetNodeID, "comment": comment})
	e.fireListeners(ctx, &Event{
		Type: EventInstanceReturned, TenantID: inst.TenantID, InstanceID: inst.ID,
		NodeID: targetNodeID, ActorID: actorID, Vars: inst.Variables.Clone(), At: e.clock.Now(),
	})

	// 自目标节点生成新令牌重走。
	nt := &Token{
		ID:         e.idgen(),
		InstanceID: inst.ID,
		NodeID:     targetNodeID,
		Status:     TokenRunning,
		CreatedAt:  e.clock.Now(),
	}
	if err := e.store.Tokens().Save(ctx, nt); err != nil {
		return err
	}
	return e.runToken(ctx, inst, nt, def)
}

// reachableFrom 判定 to 是否自 from 沿顺序流可达。
func reachableFrom(def *Definition, from, to string) bool {
	reached := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == to {
			return true
		}
		for _, e := range def.OutEdges(cur) {
			if !reached[e.To] {
				reached[e.To] = true
				queue = append(queue, e.To)
			}
		}
	}
	return false
}

// ForwardTask 转办：把待办任务移交他人。
func (e *Engine) ForwardTask(ctx context.Context, taskID, actorID, toUserID string) error {
	task, err := e.store.Tasks().Get(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status != TaskPending {
		return fmt.Errorf("%w: task %s not pending", ErrInvalidState, task.ID)
	}
	if actorID != "" && actorID != task.Assignee {
		return fmt.Errorf("%w: actor %q is not assignee", ErrForbidden, actorID)
	}
	if toUserID == "" || toUserID == task.Assignee {
		return fmt.Errorf("flow: invalid forward target")
	}
	inst, err := e.store.Instances().Get(ctx, task.InstanceID)
	if err != nil {
		return err
	}
	task.Assignee = toUserID
	if err := e.store.Tasks().Update(ctx, task); err != nil {
		return err
	}
	_ = e.appendHistory(ctx, inst, &Event{
		Type: EventTaskForwarded, NodeID: task.NodeID, TaskID: task.ID, ActorID: actorID,
	}, Vars{"to": toUserID})
	return nil
}

// Withdraw 发起人撤回。仅发起人可撤回（权限细化由宿主在上层拦截）。
func (e *Engine) Withdraw(ctx context.Context, instanceID, actorID string) error {
	inst, err := e.store.Instances().Get(ctx, instanceID)
	if err != nil {
		return err
	}
	if !inst.Status.Live() {
		return fmt.Errorf("%w: instance %s is %s", ErrInvalidState, inst.ID, inst.Status)
	}
	if inst.StarterID != "" && actorID != inst.StarterID {
		return fmt.Errorf("%w: only starter can withdraw", ErrForbidden)
	}
	return e.terminateInstance(ctx, inst, InstanceWithdrawn, actorID)
}

// Abort 管理员强制终止。不向父流程传播。
func (e *Engine) Abort(ctx context.Context, instanceID, actorID string) error {
	inst, err := e.store.Instances().Get(ctx, instanceID)
	if err != nil {
		return err
	}
	if !inst.Status.Live() {
		return fmt.Errorf("%w: instance %s is %s", ErrInvalidState, inst.ID, inst.Status)
	}
	return e.terminateInstance(ctx, inst, InstanceAborted, actorID)
}

// ============================================================
// 查询
// ============================================================

func (e *Engine) GetInstance(ctx context.Context, id string) (*Instance, error) {
	return e.store.Instances().Get(ctx, id)
}

func (e *Engine) ListInstanceTasks(ctx context.Context, instanceID string) ([]*Task, error) {
	return e.store.Tasks().ListByInstance(ctx, instanceID)
}

func (e *Engine) ListPendingTasks(ctx context.Context, tenantID, assignee string) ([]*Task, error) {
	return e.store.Tasks().ListPendingByAssignee(ctx, tenantID, assignee)
}

func (e *Engine) ListHistory(ctx context.Context, instanceID string) ([]*HistoryEvent, error) {
	return e.store.History().ListByInstance(ctx, instanceID)
}

// CheckDueTasks 扫描到期任务并广播 EventTaskDue（催办/升级/超时自动处理由监听器承接）。
// 由宿主定时任务驱动；返回本次扫描到的到期任务。
func (e *Engine) CheckDueTasks(ctx context.Context, now time.Time) ([]*Task, error) {
	due, err := e.store.Tasks().ListDue(ctx, now)
	if err != nil {
		return nil, err
	}
	for _, t := range due {
		inst, err := e.store.Instances().Get(ctx, t.InstanceID)
		if err != nil || inst == nil || !inst.Status.Live() {
			continue
		}
		_ = e.appendHistory(ctx, inst, &Event{
			Type: EventTaskDue, NodeID: t.NodeID, TaskID: t.ID,
		}, Vars{"dueAt": t.DueAt.Unix()})
	}
	return due, nil
}

// ResumeDueTimers 扫描到期定时器节点并续跑令牌（宿主 cron 驱动）。
// 与 CheckDueTasks 同一调度习惯：定时器到期是引擎自驱（令牌续走），
// 任务到期是宿主自决（监听器决定催办/升级）。
// 返回本次续跑的令牌；单个令牌推进失败立即返回（fail-closed，不静默跳过）。
func (e *Engine) ResumeDueTimers(ctx context.Context, now time.Time) ([]*Token, error) {
	due, err := e.store.Tokens().ListDueTimers(ctx, now)
	if err != nil {
		return nil, err
	}
	var resumed []*Token
	for _, tk := range due {
		inst, err := e.store.Instances().Get(ctx, tk.InstanceID)
		if err != nil || inst == nil || !inst.Status.Live() {
			continue
		}
		def, err := e.definitionOf(ctx, inst)
		if err != nil {
			return resumed, err
		}
		out := def.OutEdges(tk.NodeID)
		if len(out) != 1 {
			return resumed, fmt.Errorf("timer node %s has %d out edges", tk.NodeID, len(out))
		}

		_ = e.appendHistory(ctx, inst, &Event{Type: EventTimerFired, NodeID: tk.NodeID}, Vars{"dueAt": tk.DueAt.Unix()})
		e.fireListeners(ctx, &Event{
			Type: EventTimerFired, TenantID: inst.TenantID, InstanceID: inst.ID,
			NodeID: tk.NodeID, Vars: inst.Variables.Clone(), At: e.clock.Now(),
		})

		tk.Status = TokenRunning
		tk.NodeID = out[0].To
		tk.DueAt = nil
		if err := e.store.Tokens().Update(ctx, tk); err != nil {
			return resumed, err
		}
		resumed = append(resumed, tk)
		if err := e.runToken(ctx, inst, tk, def); err != nil {
			return resumed, err
		}
	}
	return resumed, nil
}

// ============================================================
// 内部：实例终结
// ============================================================

// rejectInstance 驳回终结：fail-closed，同步向父流程传播。
func (e *Engine) rejectInstance(ctx context.Context, inst *Instance, actorID string) error {
	return e.terminateInstance(ctx, inst, InstanceRejected, actorID)
}

// abortInstance 系统性异常终止（如定义损坏）。
func (e *Engine) abortInstance(ctx context.Context, inst *Instance, reason string) error {
	e.log.Printf("flow: abort instance %s: %s", inst.ID, reason)
	return e.terminateInstance(ctx, inst, InstanceAborted, "flow:system")
}

func (e *Engine) terminateInstance(ctx context.Context, inst *Instance, status InstanceStatus, actorID string) error {
	// 取消全部在途任务。
	tasks, _ := e.store.Tasks().ListByInstance(ctx, inst.ID)
	for _, t := range tasks {
		if t.Status == TaskPending {
			now := e.clock.Now()
			t.Status = TaskCancelled
			t.EndedAt = &now
			_ = e.store.Tasks().Update(ctx, t)
		}
	}
	// 清空在途令牌。
	tokens, _ := e.store.Tokens().ListByInstance(ctx, inst.ID)
	for _, tk := range tokens {
		if tk.Status != TokenEnded {
			tk.Status = TokenEnded
			_ = e.store.Tokens().Update(ctx, tk)
		}
	}

	now := e.clock.Now()
	inst.Status = status
	inst.EndedAt = &now
	if err := e.store.Instances().Update(ctx, inst); err != nil {
		return err
	}
	_ = e.appendHistory(ctx, inst, &Event{Type: EventProcessEnded, Status: status, ActorID: actorID}, nil)
	e.fireListeners(ctx, &Event{
		Type: EventProcessEnded, TenantID: inst.TenantID, InstanceID: inst.ID,
		Status: status, ActorID: actorID, Vars: inst.Variables.Clone(), At: e.clock.Now(),
	})

	// 子流程终结向父流程传播（fail-closed：REJECTED/WITHDRAWN/ABORTED 均令父实例驳回）。
	if inst.ParentInstanceID != "" {
		return e.resumeParentOnChildEnd(ctx, inst, status)
	}
	return nil
}

func (e *Engine) resumeParentOnChildEnd(ctx context.Context, child *Instance, status InstanceStatus) error {
	parent, err := e.store.Instances().Get(ctx, child.ParentInstanceID)
	if err != nil || parent == nil {
		return err
	}
	if !parent.Status.Live() {
		return nil
	}
	if status != InstanceApproved {
		// fail-closed：子流程未通过，父实例整体驳回。
		return e.rejectInstance(ctx, parent, "flow:subprocess")
	}
	tk, err := e.store.Tokens().Get(ctx, child.ParentTokenID)
	if err != nil {
		return err
	}
	def, err := e.definitionOf(ctx, parent)
	if err != nil {
		return err
	}
	// 令牌离开 callActivity 节点（沿唯一出边推进），否则会再次进入子流程。
	if out := def.OutEdges(tk.NodeID); len(out) == 1 {
		tk.NodeID = out[0].To
	}
	tk.Status = TokenRunning
	if err := e.store.Tokens().Update(ctx, tk); err != nil {
		return err
	}
	return e.runToken(ctx, parent, tk, def)
}

// ============================================================
// 内部：辅助
// ============================================================

func (e *Engine) definitionOf(ctx context.Context, inst *Instance) (*Definition, error) {
	return e.store.Definitions().Get(ctx, inst.TenantID, inst.DefinitionKey, inst.DefinitionVersion)
}

func (e *Engine) appendHistory(ctx context.Context, inst *Instance, ev *Event, payload Vars) error {
	hv := &HistoryEvent{
		ID:         e.idgen(),
		InstanceID: inst.ID,
		Type:       ev.Type,
		NodeID:     ev.NodeID,
		TaskID:     ev.TaskID,
		ActorID:    ev.ActorID,
		Payload:    payload,
		CreatedAt:  e.clock.Now(),
	}
	if err := e.store.History().Append(ctx, hv); err != nil {
		e.log.Printf("flow: append history failed: %v", err)
		return err
	}
	return nil
}

func (e *Engine) fireListeners(ctx context.Context, ev *Event) {
	for _, l := range e.listeners {
		func() {
			defer func() { _ = recover() }()
			l.OnFlowEvent(ctx, ev)
		}()
	}
}

func findStart(def *Definition) string {
	for id, n := range def.Nodes {
		if n.Type == NodeStartEvent {
			return id
		}
	}
	return ""
}

func mapOutcome(outcome string) TaskStatus {
	if outcome == OutcomeReject {
		return TaskRejected
	}
	return TaskApproved
}

func diff(after, before []*Task) []*Task {
	seen := map[string]bool{}
	for _, t := range before {
		seen[t.ID] = true
	}
	var out []*Task
	for _, t := range after {
		if !seen[t.ID] {
			out = append(out, t)
		}
	}
	return out
}

func taskIDs(tasks []*Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	return ids
}
