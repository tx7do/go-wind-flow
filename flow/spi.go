package flow

import (
	"context"
	"time"
)

// ============================================================
// SPI：核心引擎的全部外部依赖。宿主按需装配，核心零第三方依赖。
// ============================================================

// ExpressionEvaluator 布尔表达式求值器（网关条件、动态办理人等）。
//
// 约定：
//   - 表达式为裸表达式（BPMN conditionExpression 的 ${...} 由导入器剥壳）；
//   - 求值必须无副作用、确定性、禁止 I/O（安全要求）；
//   - 求值失败返回 error，引擎一律 fail-closed（不误走分支）。
//
// 内置 SimpleEvaluator 覆盖常见比较/逻辑运算；生产可换 contrib/exprlang
// （expr-lang，与 go-wind-oa/im 现行为一致）或 go-scripts 的 CEL/Expr 引擎。
type ExpressionEvaluator interface {
	EvalBool(ctx context.Context, expression string, env Vars) (bool, error)
}

// ScriptRunner 脚本执行器（脚本任务与脚本监听器）。
// 实现方负责沙箱、超时（context deadline）与并发隔离（引擎池）。
type ScriptRunner interface {
	Run(ctx context.Context, spec *ScriptSpec, env Vars) (any, error)
}

// ResolveRequest 办理人解析请求。
type ResolveRequest struct {
	TenantID     string
	InstanceID   string
	DefinitionID string
	NodeID       string
	StarterID    string
	InstanceVars Vars
	Rules        []ApproverRule
}

// IdentityProvider 身份解析 SPI：把 ApproverRule 解析为具体用户 ID 列表，
// 并提供委托（delegation）代办的叠加解析。
//
// 静态规则（USER/STARTER）核心内置可解析；动态规则（LEADER/POSITION/ROLE/EXPR）
// 必须由宿主实现本接口，未装配时引擎 fail-closed 报 ErrNotConfigured。
type IdentityProvider interface {
	ResolveApprovers(ctx context.Context, req ResolveRequest) ([]string, error)
	// ResolveDelegate 返回 userID 的受托人；无委托时返回 userID 本身。
	ResolveDelegate(ctx context.Context, tenantID, userID string) (string, error)
}

// Store 持久化 SPI。每次状态迁移引擎都会调用对应写方法，
// 实现方应保证：同实例内调用按发生顺序可见（ 单实例串行）。
// 生产实现（ent）建议在每个引擎入口（Start/CompleteTask/...）包一层事务。
type Store interface {
	Definitions() DefinitionStore
	Instances() InstanceStore
	Tokens() TokenStore
	Tasks() TaskStore
	History() HistoryStore
}

// DefinitionStore 定义存储。按 (tenantID, key, version) 唯一。
type DefinitionStore interface {
	Save(ctx context.Context, def *Definition) error
	// Get version<=0 表示取最新版本。
	Get(ctx context.Context, tenantID, key string, version int) (*Definition, error)
	MaxVersion(ctx context.Context, tenantID, key string) (int, error)
}

// InstanceStore 实例存储。
type InstanceStore interface {
	Save(ctx context.Context, inst *Instance) error
	Get(ctx context.Context, id string) (*Instance, error)
	Update(ctx context.Context, inst *Instance) error
	ListByStatus(ctx context.Context, status InstanceStatus) ([]*Instance, error)
}

// TokenStore 令牌存储。
type TokenStore interface {
	Save(ctx context.Context, tk *Token) error
	Update(ctx context.Context, tk *Token) error
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*Token, error)
	ListByInstance(ctx context.Context, instanceID string) ([]*Token, error)
}

// TaskStore 任务存储。
type TaskStore interface {
	Save(ctx context.Context, t *Task) error
	Update(ctx context.Context, t *Task) error
	Get(ctx context.Context, id string) (*Task, error)
	ListByInstance(ctx context.Context, instanceID string) ([]*Task, error)
	ListByToken(ctx context.Context, tokenID string) ([]*Task, error)
	ListPendingByAssignee(ctx context.Context, tenantID, assignee string) ([]*Task, error)
	ListDue(ctx context.Context, now time.Time) ([]*Task, error)
}

// HistoryStore 历史存储（append-only）。
type HistoryStore interface {
	Append(ctx context.Context, ev *HistoryEvent) error
	ListByInstance(ctx context.Context, instanceID string) ([]*HistoryEvent, error)
}
