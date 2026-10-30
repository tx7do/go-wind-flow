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

// TimerSpec 超时规格：任务创建后 DueAt = now + Duration。
// 引擎只负责打标与 EventTaskDue 事件，调度由宿主 cron 驱动 CheckDueTasks。
type TimerSpec struct {
	Duration Duration `json:"duration,omitempty"` // 如 "24h"、"30m"
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
type ScriptSpec struct {
	Lang string `json:"lang,omitempty"`
	Code string `json:"code,omitempty"`
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

	Documentation string `json:"documentation,omitempty"`
}

// ApprovalSpec 审批行为配置。
type ApprovalSpec struct {
	Strategy   Strategy   `json:"strategy,omitempty"`   // 缺省 = ALL 会签
	RejectMode RejectMode `json:"rejectMode,omitempty"` // 缺省 = TERMINATE
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
