package flow

import (
	"strings"
	"time"
)

// NodeType 节点类型。命名对齐 BPMN 2.0 元素，便于与 BPMN 工具链互认。
type NodeType string

const (
	NodeStartEvent       NodeType = "startEvent"
	NodeEndEvent         NodeType = "endEvent"
	NodeUserTask         NodeType = "userTask"
	NodeScriptTask       NodeType = "scriptTask"
	NodeServiceTask      NodeType = "serviceTask" // v0 与 ScriptTask 同语义（回调由宿主监听器承接）
	NodeTimerEvent       NodeType = "intermediateTimerEvent" // 中间定时器：令牌阻塞至到期，宿主 cron 驱动续跑
	NodeMessageEvent     NodeType = "intermediateMessageEvent" // 中间消息：令牌阻塞等外部投递（SendMessage 解锁）
	NodeErrorEndEvent    NodeType = "errorEndEvent"          // 错误结束：终结实例并抛出错误码（父 callActivity 错误边界可捕获）
	NodeBoundaryEvent    NodeType = "boundaryEvent"          // 边界事件：挂接在 userTask/callActivity 上，到期或错误触发升级/接管分支
	NodeExclusiveGateway NodeType = "exclusiveGateway"
	NodeParallelGateway  NodeType = "parallelGateway" // 分叉/汇合同型，按出入度区分（BPMN 惯例）
	NodeInclusiveGateway NodeType = "inclusiveGateway"
	NodeCallActivity     NodeType = "callActivity" // 子流程
)

// Strategy 多实例审批策略（对应 BPMN multiInstanceLoopCharacteristics 的完成语义）。
type Strategy string

const (
	// StrategyAll 会签：全员通过才推进，任一驳回即驳回（默认，fail-closed）。
	StrategyAll Strategy = "ALL"
	// StrategyAny 或签：任一通过即推进（其余任务取消），全员驳回才驳回。
	StrategyAny Strategy = "ANY"
)

// RejectMode 驳回语义。
type RejectMode string

const (
	// RejectTerminate 驳回即终结实例（REJECTED），与 go-wind-oa/im 现行为一致（默认）。
	RejectTerminate RejectMode = "TERMINATE"
	// RejectStay 驳回后停留（任务标 REJECTED，实例继续），留给宿主自行接线后续处理。
	RejectStay RejectMode = "STAY"
)

// ApproverKind 办理人规则类型。静态类型（USER/STARTER）核心内置可解析；
// 动态类型（LEADER/POSITION/ROLE/EXPR）要求宿主装配 IdentityProvider。
type ApproverKind string

const (
	ApproverUser     ApproverKind = "USER"     // Value = 用户 ID
	ApproverStarter  ApproverKind = "STARTER"  // 发起人
	ApproverLeader   ApproverKind = "LEADER"   // 发起人主组织负责人
	ApproverPosition ApproverKind = "POSITION" // Value = 职位编码
	ApproverRole     ApproverKind = "ROLE"     // Value = 角色编码
	ApproverExpr     ApproverKind = "EXPR"     // Value = 表达式（返回用户 ID 或 ID 数组）
	ApproverGroup    ApproverKind = "GROUP"    // Value = 组编码：创建候选组任务，组内成员认领后办理
)

// ApproverRule 单条办理人规则。
type ApproverRule struct {
	Kind  ApproverKind `json:"kind"`
	Value string       `json:"value,omitempty"`
}

// ApproverSpec 办理人规格：多条规则依序解析、去重保序。
type ApproverSpec struct {
	Rules []ApproverRule `json:"rules,omitempty"`
}

// TimerAction 任务到期动作（缺省仅广播 task.due，由监听器自决）。
type TimerAction string

const (
	TimerActionNone        TimerAction = ""
	TimerActionAutoApprove TimerAction = "auto_approve" // 到期自动通过（系统操作者 flow:timer）
	TimerActionAutoReject  TimerAction = "auto_reject"  // 到期自动驳回（按节点 RejectMode 终结/停留）
)

// TimerSpec 超时规格：任务创建后 DueAt = now + Duration。
// 调度由宿主 cron 驱动 CheckDueTasks：Action 为空时仅广播 task.due 事件
//（催办/升级由监听器承接），非空时引擎直接执行对应动作。
type TimerSpec struct {
	Duration Duration     `json:"duration,omitempty"` // 如 "24h"、"30m"
	Action   TimerAction  `json:"action,omitempty"`   // "" | auto_approve | auto_reject
}

// Duration 兼容 JSON 字符串（"24h"）与数值秒的时间跨度。
type Duration time.Duration

func (d Duration) Std() time.Duration { return time.Duration(d) }

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Duration(d).String() + `"`), nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

// ScriptSpec 脚本任务/监听器脚本规格。Lang 由 ScriptRunner 实现方约定
// （contrib/goscripts: js/lua/python/go/cel/expr...）。
//
// Scope 决定脚本返回 map 的落点：
//   - "" / "instance"（默认）→ 实例变量（业务数据可见）；
//   - "local" → 令牌级局部变量（仅当前分支的后续节点可读，不污染业务数据）。
type ScriptSpec struct {
	Lang  string `json:"lang,omitempty"`
	Code  string `json:"code,omitempty"`
	Scope string `json:"scope,omitempty"` // "" | "instance" | "local"
}

// Node 定义图中的节点。
type Node struct {
	ID   string   `json:"id"`
	Name string   `json:"name,omitempty"`
	Type NodeType `json:"type"`

	// UserTask
	Approver *ApproverSpec `json:"approver,omitempty"`
	Approval ApprovalSpec  `json:"approval,omitempty"`
	Timer    *TimerSpec    `json:"timer,omitempty"`

	// ScriptTask / ServiceTask
	Script *ScriptSpec `json:"script,omitempty"`

	// CallActivity
	SubprocessKey     string `json:"subprocessKey,omitempty"`
	SubprocessVersion int    `json:"subprocessVersion,omitempty"` // 0 = latest

	// ExclusiveGateway 默认出边 ID（无条件命中时兜底，fail-closed：必须存在）。
	DefaultEdge string `json:"defaultEdge,omitempty"`

	// BoundaryEvent 专用：挂接的目标 userTask/callActivity；Interrupting 缺省
	// true（到期取消原任务并接管流程），false = 非中断（只派生升级分支）。
	// ErrorCode 非空 = 错误边界（捕获 callActivity 子流程抛出的错误码，
	// 空串通配）；Timer 非空 = 定时边界。
	AttachedTo   string `json:"attachedTo,omitempty"`
	Interrupting *bool  `json:"interrupting,omitempty"`
	ErrorCode    string `json:"errorCode,omitempty"`

	// MessageEvent 专用：消息名（SendMessage 按实例 + 消息名投递解锁）。
	Message string `json:"message,omitempty"`

	Documentation string `json:"documentation,omitempty"`
}

// ApprovalSpec 审批行为配置。
type ApprovalSpec struct {
	Strategy   Strategy   `json:"strategy,omitempty"`   // 缺省 = ALL 会签
	RejectMode RejectMode `json:"rejectMode,omitempty"` // 缺省 = TERMINATE
	// Sequential 顺序多实例（BPMN isSequential=true）：办理人列表串行逐个审批
	//（按解析顺序），全部通过才推进；中途驳回按 RejectMode。串行隐含 ALL 语义。
	Sequential bool `json:"sequential,omitempty"`
	// Collection 多实例集合表达式（求值结果须为字符串数组）：
	//   - 并行多实例：每项一个任务（项即办理人，ElementVar 不生效）；
	//   - 顺序多实例 + ElementVar：串行逐项，项写入令牌局部变量（非保留键），
	//     后续节点/条件可读；ElementVar 缺省 "item"。
	// 设置后覆盖 Approver 规则解析。
	Collection string `json:"collection,omitempty"`
	ElementVar string `json:"elementVar,omitempty"`
}

// Edge 定义图中的顺序流。
type Edge struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Name      string `json:"name,omitempty"`
	Condition string `json:"condition,omitempty"` // 布尔表达式（裸表达式，非 UEL 包裹）
	IsDefault bool   `json:"isDefault,omitempty"` // 排他网关默认流
}

// Definition 一份已部署的流程定义。不可变：部署生成新版本，修改需重新部署。
type Definition struct {
	Key       string
	Name      string
	Version   int
	TenantID  string
	Format    string // 来源格式：bpmn-xml / wind-json-v2 / 自定义
	Source    []byte // 原始定义文本（审计与导出用）
	CreatedAt time.Time

	Nodes map[string]*Node
	Edges []*Edge
}

// InDegree 节点入度。
func (d *Definition) InDegree(nodeID string) int {
	n := 0
	for _, e := range d.Edges {
		if e.To == nodeID {
			n++
		}
	}
	return n
}

// OutEdges 返回节点出边（保持定义顺序）。
func (d *Definition) OutEdges(nodeID string) []*Edge {
	var out []*Edge
	for _, e := range d.Edges {
		if e.From == nodeID {
			out = append(out, e)
		}
	}
	return out
}

// Edge 返回指定 ID 的边。
func (d *Definition) Edge(id string) *Edge {
	for _, e := range d.Edges {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// IsInterrupting 边界事件是否中断型（缺省 true，BPMN cancelActivity 默认语义）。
func (n *Node) IsInterrupting() bool {
	if n.Interrupting == nil {
		return true
	}
	return *n.Interrupting
}

// Clone 深拷贝（部署侧内存复用需隔离时使用）。
func (d *Definition) Clone() *Definition {
	cp := *d
	cp.Nodes = make(map[string]*Node, len(d.Nodes))
	for k, v := range d.Nodes {
		node := *v
		cp.Nodes[k] = &node
	}
	cp.Edges = make([]*Edge, len(d.Edges))
	for i, e := range d.Edges {
		edge := *e
		cp.Edges[i] = &edge
	}
	cp.Source = append([]byte(nil), d.Source...)
	return &cp
}
