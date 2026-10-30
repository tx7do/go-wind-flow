package flow

import (
	"context"
	"time"
)

// Vars 变量环境：实例级变量 + 表单数据的统一命名空间。
// 表达式、脚本、监听器共享同一环境；键名建议采用业务字段名（如 days、amount）。
type Vars map[string]any

// Clone 深拷贝（只拷贝顶层，值 presumed 不可变）。
func (v Vars) Clone() Vars {
	cp := make(Vars, len(v))
	for k, val := range v {
		cp[k] = val
	}
	return cp
}

// Merge 将 src 合并进 v（src 优先），返回 v。
func (v Vars) Merge(src Vars) Vars {
	for k, val := range src {
		v[k] = val
	}
	return v
}

// InstanceStatus 实例状态。
type InstanceStatus string

const (
	InstanceRunning   InstanceStatus = "RUNNING"
	InstanceApproved  InstanceStatus = "APPROVED"  // 正常走完 END
	InstanceRejected  InstanceStatus = "REJECTED"  // 驳回终结（fail-closed）
	InstanceWithdrawn InstanceStatus = "WITHDRAWN" // 发起人撤回
	InstanceAborted   InstanceStatus = "ABORTED"   // 管理员强制终止
)

// Live 报告实例是否仍在运行。
func (s InstanceStatus) Live() bool { return s == InstanceRunning }

// TokenStatus 令牌状态。
type TokenStatus string

const (
	TokenRunning TokenStatus = "RUNNING" // 正在推进
	TokenWaiting TokenStatus = "WAITING" // 阻塞于用户任务/子流程/汇合点
	TokenEnded   TokenStatus = "ENDED"
)

// TaskStatus 任务状态。
type TaskStatus string

const (
	TaskPending   TaskStatus = "PENDING"
	TaskApproved  TaskStatus = "APPROVED"
	TaskRejected  TaskStatus = "REJECTED"
	TaskCancelled TaskStatus = "CANCELLED"
	TaskForwarded TaskStatus = "FORWARDED"
)

// Outcome 办结动作（v0 两个值，后续可扩展转办/退回指定节点等）。
const (
	OutcomeApprove = "APPROVE"
	OutcomeReject  = "REJECT"
)

// Instance 流程实例。
type Instance struct {
	ID                string
	TenantID          string
	DefinitionKey     string
	DefinitionVersion int
	BusinessKey       string // 业务单据 ID
	BusinessType      string // 业务单据类型（监听器路由用）
	StarterID         string
	Status            InstanceStatus
	Variables         Vars
	// 子流程链接：本实例由父实例的 ParentTokenID 令牌在 callActivity 节点发起。
	ParentInstanceID string
	ParentTokenID    string
	CreatedAt        time.Time
	EndedAt          *time.Time
}

// Token 执行令牌：实例在图中的游标。
type Token struct {
	ID          string
	InstanceID  string
	NodeID      string
	Status      TokenStatus
	ParentID    string     // fork 产生的子令牌指向分叉源令牌
	ChildInstID string     // 阻塞于 callActivity 时记录子流程实例
	DueAt       *time.Time // 阻塞于定时器节点时的到期时刻
	// Locals 令牌级局部变量（变量作用域）：并行分支各持一份（fork 时快照
	// 克隆），读取时叠加在实例变量之上（同名覆盖），不回写业务数据。
	Locals    Vars
	CreatedAt time.Time
}

// Task 用户任务。
type Task struct {
	ID         string
	InstanceID string
	TenantID   string
	NodeID     string
	TokenID    string
	Name       string
	Assignee   string
	Status     TaskStatus
	Outcome    string
	Comment    string
	DueAt      *time.Time
	CreatedAt  time.Time
	EndedAt    *time.Time
}

// HistoryEvent 流转历史（append-only，审计与时间线渲染）。
type HistoryEvent struct {
	ID         string
	InstanceID string
	Type       EventType
	NodeID     string
	TaskID     string
	ActorID    string
	Payload    Vars
	CreatedAt  time.Time
}

// EventType 生命周期事件类型（历史记录与监听器共用）。
type EventType string

const (
	EventProcessStarted    EventType = "process.started"
	EventProcessEnded      EventType = "process.ended"
	EventTokenMoved        EventType = "token.moved"
	EventGatewayDecided    EventType = "gateway.decided"
	EventTaskCreated       EventType = "task.created"
	EventTaskCompleted     EventType = "task.completed"
	EventTaskForwarded     EventType = "task.forwarded"
	EventTaskApproverAdded EventType = "task.approver_added" // 加签
	EventTaskDue           EventType = "task.due"
	EventTimerFired        EventType = "timer.fired"       // 定时器节点到期续跑
	EventInstanceReturned  EventType = "instance.returned" // 退回指定节点
	EventVariableChanged   EventType = "variable.changed"
)

// Event 监听器事件。
type Event struct {
	Type       EventType
	TenantID   string
	InstanceID string
	TokenID    string
	TaskID     string
	NodeID     string
	ActorID    string
	// Status 流程终态（仅 EventProcessEnded 时有值）。
	Status InstanceStatus
	// Vars 事件时刻的实例变量快照（只读约定）。
	Vars Vars
	At   time.Time
}

// Listener 监听器。回调 panic 由引擎 recover 保护，不影响状态机结果。
type Listener interface {
	OnFlowEvent(ctx context.Context, ev *Event)
}

// ListenerFunc 函数式监听器适配。
type ListenerFunc func(ctx context.Context, ev *Event)

func (f ListenerFunc) OnFlowEvent(ctx context.Context, ev *Event) { f(ctx, ev) }
