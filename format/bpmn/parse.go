// Package bpmn 提供 BPMN 2.0 XML 与 flow.Definition 的双向转换，
// 是 go-wind-flow 与 BPMN 生态（bpmn-js、Camunda Modeler 等）互通的桥梁。
//
// 支持子集与扩展约定见 docs/bpmn-compat.md。
package bpmn

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
)

// FormatName 注册到引擎的格式标识。
const FormatName = "bpmn-xml"

// NsFlow go-wind-flow 扩展元素命名空间（approvers/timer 等自定义载荷）。
const NsFlow = "https://github.com/tx7do/go-wind-flow"

// Importer 实现 flow.Importer。
type Importer struct{}

func (Importer) Format() string { return FormatName }

func (Importer) Import(data []byte) (*flow.Definition, error) { return Parse(data) }

// Exporter 实现 flow.Exporter。
type Exporter struct{}

func (Exporter) Format() string { return FormatName }

func (Exporter) Export(def *flow.Definition) ([]byte, error) { return Write(def) }

// ============================================================
// 解析
// ============================================================

type definitionsXML struct {
	XMLName   xml.Name     `xml:"definitions"`
	Processes []processXML `xml:"process"`
}

type processXML struct {
	ID           string `xml:"id,attr"`
	Name         string `xml:"name,attr"`
	IsExecutable string `xml:"isExecutable,attr"`

	StartEvents    []eventXML      `xml:"startEvent"`
	EndEvents      []eventXML      `xml:"endEvent"`
	UserTasks      []userTaskXML   `xml:"userTask"`
	ScriptTasks    []scriptTaskXML `xml:"scriptTask"`
	ServiceTasks   []scriptTaskXML `xml:"serviceTask"`
	TimerEvents    []catchEventXML `xml:"intermediateCatchEvent"`
	BoundaryEvents []boundaryXML   `xml:"boundaryEvent"`

	ExclusiveGateways []gatewayXML `xml:"exclusiveGateway"`
	ParallelGateways  []gatewayXML `xml:"parallelGateway"`
	InclusiveGateways []gatewayXML `xml:"inclusiveGateway"`

	CallActivities []callXML `xml:"callActivity"`

	SubProcesses []subProcessXML `xml:"subProcess"`

	SequenceFlows []seqFlowXML `xml:"sequenceFlow"`
}

type eventXML struct {
	ID    string       `xml:"id,attr"`
	Name  string       `xml:"name,attr"`
	Doc   string       `xml:"documentation"`
	Error *errorDefXML `xml:"errorEventDefinition"`
	Timer *timerDefXML `xml:"timerEventDefinition"`
}

type errorDefXML struct {
	ErrorRef string `xml:"errorRef,attr"`
}

// subProcessXML 嵌入子流程。v1 仅支持扁平化导入：内部恰好一个普通 start
// 与一个普通 end（单入单出），导入时以 `<subProcessID>.<childID>` 前缀
// 并入父图；带事件子流程/边界/多实例的嵌入子流程显式拒绝。
type subProcessXML struct {
	ID           string          `xml:"id,attr"`
	Name         string          `xml:"name,attr"`
	StartEvents  []eventXML      `xml:"startEvent"`
	EndEvents    []eventXML      `xml:"endEvent"`
	UserTasks    []userTaskXML   `xml:"userTask"`
	ScriptTasks  []scriptTaskXML `xml:"scriptTask"`
	ServiceTasks []scriptTaskXML `xml:"serviceTask"`

	ExclusiveGateways []gatewayXML `xml:"exclusiveGateway"`
	ParallelGateways  []gatewayXML `xml:"parallelGateway"`

	SequenceFlows []seqFlowXML `xml:"sequenceFlow"`
}

type userTaskXML struct {
	ID              string    `xml:"id,attr"`
	Name            string    `xml:"name,attr"`
	Doc             string    `xml:"documentation"`
	Assignee        string    `xml:"assignee,attr"`
	CandidateUsers  string    `xml:"candidateUsers,attr"`
	CandidateGroups string    `xml:"candidateGroups,attr"`
	Extension       *extXML   `xml:"extensionElements"`
	MultiInstance   *multiXML `xml:"multiInstanceLoopCharacteristics"`
}

type extXML struct {
	Inner string `xml:",innerxml"`
}

type multiXML struct {
	IsSequential        string `xml:"isSequential,attr"`
	CompletionCondition string `xml:"completionCondition"`
}

type scriptTaskXML struct {
	ID           string `xml:"id,attr"`
	Name         string `xml:"name,attr"`
	Doc          string `xml:"documentation"`
	ScriptFormat string `xml:"scriptFormat,attr"`
	// Scope go-wind-flow 扩展："" / instance / local。
	// 前缀声明与否会改变 Go xml 的属性名解析（Local=scope + Space=URL vs
	// 字面 "flow:scope"），两个字段都接，取先到。
	ScopeNS string         `xml:"https://github.com/tx7do/go-wind-flow scope,attr"`
	Scope   string         `xml:"flow:scope,attr"`
	Script  *scriptBodyXML `xml:"script"`
}

// scopeAttr 归一化后的作用域。
func (t scriptTaskXML) scopeAttr() string {
	if t.ScopeNS != "" {
		return t.ScopeNS
	}
	return t.Scope
}

type scriptBodyXML struct {
	Inner string `xml:",innerxml"`
}

// catchEventXML 中间捕获事件（定时器 / 消息定义）。
type catchEventXML struct {
	ID      string         `xml:"id,attr"`
	Name    string         `xml:"name,attr"`
	Doc     string         `xml:"documentation"`
	Timer   *timerDefXML   `xml:"timerEventDefinition"`
	Message *messageDefXML `xml:"messageEventDefinition"`
}

type messageDefXML struct {
	MessageRef string `xml:"messageRef,attr"`
}

type timerDefXML struct {
	Duration string `xml:"timeDuration"`
}

// boundaryXML 边界事件（当前支持定时器触发，挂接 userTask）。
type boundaryXML struct {
	ID             string       `xml:"id,attr"`
	Name           string       `xml:"name,attr"`
	Doc            string       `xml:"documentation"`
	AttachedToRef  string       `xml:"attachedToRef,attr"`
	CancelActivity *string      `xml:"cancelActivity,attr"` // 缺省 true
	Timer          *timerDefXML `xml:"timerEventDefinition"`
	Error          *errorDefXML `xml:"errorEventDefinition"`
}

type gatewayXML struct {
	ID      string `xml:"id,attr"`
	Name    string `xml:"name,attr"`
	Doc     string `xml:"documentation"`
	Default string `xml:"default,attr"`
}

type callXML struct {
	ID            string `xml:"id,attr"`
	Name          string `xml:"name,attr"`
	Doc           string `xml:"documentation"`
	CalledElement string `xml:"calledElement,attr"`
}

type seqFlowXML struct {
	ID        string   `xml:"id,attr"`
	SourceRef string   `xml:"sourceRef,attr"`
	TargetRef string   `xml:"targetRef,attr"`
	Name      string   `xml:"name,attr"`
	Condition *condXML `xml:"conditionExpression"`
}

type condXML struct {
	Text string `xml:",chardata"`
}

// extSpec go-wind-flow 扩展载荷（<flow:assignees> 元素内的 JSON 文本）。
type extSpec struct {
	Approvers  []flow.ApproverRule `json:"approvers,omitempty"`
	Strategy   string              `json:"strategy,omitempty"`
	RejectMode string              `json:"rejectMode,omitempty"`
	Due        string              `json:"due,omitempty"`
	Action     string              `json:"action,omitempty"`
	Collection string              `json:"collection,omitempty"` // 多实例集合表达式
	ElementVar string              `json:"elementVar,omitempty"` // 项变量名（缺省 item）
}

var tagStripper = regexp.MustCompile(`<[^>]*>`)

// Parse BPMN 2.0 XML → Definition。
func Parse(data []byte) (*flow.Definition, error) {
	var doc definitionsXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse bpmn xml: %w", err)
	}
	if len(doc.Processes) == 0 {
		return nil, fmt.Errorf("%w: no <process> in bpmn xml", flow.ErrInvalidDefinition)
	}
	p := doc.Processes[0]

	def := &flow.Definition{
		Name:  p.Name,
		Nodes: map[string]*flow.Node{},
		Edges: []*flow.Edge{},
	}
	add := func(n *flow.Node) {
		def.Nodes[n.ID] = n
	}

	for _, s := range p.StartEvents {
		node := &flow.Node{ID: s.ID, Name: s.Name, Type: flow.NodeStartEvent, Documentation: s.Doc}
		if s.Timer != nil && strings.TrimSpace(s.Timer.Duration) != "" {
			// 定时器启动事件：解析保留（ISO8601），运行期自动调度由宿主
			// cron + Start 组合驱动（引擎提供解析与查询契约）。
			d, err := ParseISODuration(s.Timer.Duration)
			if err != nil {
				return nil, fmt.Errorf("startEvent %s: %w", s.ID, err)
			}
			node.Timer = &flow.TimerSpec{Duration: flow.Duration(d)}
		}
		add(node)
	}
	for _, s := range p.EndEvents {
		if s.Error != nil {
			add(&flow.Node{
				ID: s.ID, Name: s.Name, Type: flow.NodeErrorEndEvent,
				ErrorCode: strings.TrimSpace(s.Error.ErrorRef), Documentation: s.Doc,
			})
			continue
		}
		add(&flow.Node{ID: s.ID, Name: s.Name, Type: flow.NodeEndEvent, Documentation: s.Doc})
	}
	for _, t := range p.UserTasks {
		node := &flow.Node{ID: t.ID, Name: t.Name, Type: flow.NodeUserTask}
		node.Documentation = t.Doc
		spec := parseExt(t.Extension)
		rules := spec.Approvers
		if len(rules) == 0 {
			// 标准/Activiti 风格属性兜底。
			if t.Assignee != "" {
				rules = append(rules, flow.ApproverRule{Kind: flow.ApproverUser, Value: t.Assignee})
			}
			for _, uid := range strings.Split(t.CandidateUsers, ",") {
				if uid = strings.TrimSpace(uid); uid != "" {
					rules = append(rules, flow.ApproverRule{Kind: flow.ApproverUser, Value: uid})
				}
			}
		}
		// candidateGroups（BPMN 标准属性）→ GROUP 规则（与扩展 JSON 并存时叠加）。
		for _, g := range strings.Split(t.CandidateGroups, ",") {
			if g = strings.TrimSpace(g); g != "" {
				rules = append(rules, flow.ApproverRule{Kind: flow.ApproverGroup, Value: g})
			}
		}
		node.Approver = &flow.ApproverSpec{Rules: rules}
		node.Approval = approvalFrom(spec, t.MultiInstance)
		if spec.Due != "" {
			if d, err := parseDuration(spec.Due); err == nil {
				node.Timer = &flow.TimerSpec{Duration: flow.Duration(d), Action: flow.TimerAction(spec.Action)}
			}
		}
		add(node)
	}
	for _, t := range p.ScriptTasks {
		node := &flow.Node{ID: t.ID, Name: t.Name, Type: flow.NodeScriptTask}
		node.Documentation = t.Doc
		if t.Script != nil {
			node.Script = &flow.ScriptSpec{Lang: t.ScriptFormat, Code: strings.TrimSpace(t.Script.Inner), Scope: t.scopeAttr()}
		}
		add(node)
	}
	for _, t := range p.ServiceTasks {
		node := &flow.Node{ID: t.ID, Name: t.Name, Type: flow.NodeServiceTask}
		node.Documentation = t.Doc
		if t.Script != nil {
			node.Script = &flow.ScriptSpec{Lang: t.ScriptFormat, Code: strings.TrimSpace(t.Script.Inner), Scope: t.scopeAttr()}
		}
		add(node)
	}
	for _, ev := range p.TimerEvents {
		node := &flow.Node{ID: ev.ID, Name: ev.Name}
		node.Documentation = ev.Doc
		switch {
		case ev.Timer != nil:
			node.Type = flow.NodeTimerEvent
			if strings.TrimSpace(ev.Timer.Duration) == "" {
				return nil, fmt.Errorf("%w: intermediateCatchEvent %s missing timeDuration",
					flow.ErrInvalidDefinition, ev.ID)
			}
			d, err := ParseISODuration(ev.Timer.Duration)
			if err != nil {
				return nil, fmt.Errorf("intermediateCatchEvent %s: %w", ev.ID, err)
			}
			node.Timer = &flow.TimerSpec{Duration: flow.Duration(d)}
		case ev.Message != nil:
			node.Type = flow.NodeMessageEvent
			node.Message = strings.TrimSpace(ev.Message.MessageRef)
			if node.Message == "" {
				return nil, fmt.Errorf("%w: intermediateCatchEvent %s missing messageRef",
					flow.ErrInvalidDefinition, ev.ID)
			}
		default:
			return nil, fmt.Errorf("%w: intermediateCatchEvent %s has no supported eventDefinition (timer/message)",
				flow.ErrInvalidDefinition, ev.ID)
		}
		add(node)
	}
	for _, be := range p.BoundaryEvents {
		node := &flow.Node{ID: be.ID, Name: be.Name, Type: flow.NodeBoundaryEvent, AttachedTo: be.AttachedToRef}
		node.Documentation = be.Doc
		interrupting := be.CancelActivity == nil || strings.EqualFold(strings.TrimSpace(*be.CancelActivity), "true")
		node.Interrupting = &interrupting
		if be.Error != nil {
			// 错误边界：捕获挂接节点（callActivity）子流程抛出的错误码。
			node.ErrorCode = strings.TrimSpace(be.Error.ErrorRef)
			add(node)
			continue
		}
		if be.Timer == nil || strings.TrimSpace(be.Timer.Duration) == "" {
			return nil, fmt.Errorf("%w: boundaryEvent %s missing timerEventDefinition/timeDuration",
				flow.ErrInvalidDefinition, be.ID)
		}
		d, err := ParseISODuration(be.Timer.Duration)
		if err != nil {
			return nil, fmt.Errorf("boundaryEvent %s: %w", be.ID, err)
		}
		node.Timer = &flow.TimerSpec{Duration: flow.Duration(d)}
		add(node)
	}
	for _, g := range p.ExclusiveGateways {
		add(&flow.Node{ID: g.ID, Name: g.Name, Type: flow.NodeExclusiveGateway, DefaultEdge: g.Default, Documentation: g.Doc})
	}
	for _, g := range p.ParallelGateways {
		add(&flow.Node{ID: g.ID, Name: g.Name, Type: flow.NodeParallelGateway, Documentation: g.Doc})
	}
	for _, g := range p.InclusiveGateways {
		add(&flow.Node{ID: g.ID, Name: g.Name, Type: flow.NodeInclusiveGateway, Documentation: g.Doc})
	}
	for _, c := range p.CallActivities {
		key, version := splitCalledElement(c.CalledElement)
		add(&flow.Node{
			ID: c.ID, Name: c.Name, Type: flow.NodeCallActivity,
			SubprocessKey: key, SubprocessVersion: version,
			Documentation: c.Doc,
		})
	}

	// 顺序流。
	defaults := map[string]string{} // sourceID → default edge id
	for _, g := range p.ExclusiveGateways {
		if g.Default != "" {
			defaults[g.ID] = g.Default
		}
	}
	for i, f := range p.SequenceFlows {
		edge := &flow.Edge{
			ID: f.ID, From: f.SourceRef, To: f.TargetRef, Name: f.Name,
		}
		if edge.ID == "" {
			edge.ID = "flow_" + strconv.Itoa(i+1)
		}
		if f.Condition != nil {
			edge.Condition = stripUEL(f.Condition.Text)
		}
		if defaults[f.SourceRef] == edge.ID {
			edge.IsDefault = true
		}
		def.Edges = append(def.Edges, edge)
	}

	// 嵌入子流程扁平化（SES 约束，见 subprocess.go）。
	for _, sp := range p.SubProcesses {
		if err := flattenSubProcess(def, sp); err != nil {
			return nil, err
		}
	}

	return def, nil
}

func parseExt(e *extXML) extSpec {
	var spec extSpec
	if e == nil || strings.TrimSpace(e.Inner) == "" {
		return spec
	}
	text := strings.TrimSpace(tagStripper.ReplaceAllString(e.Inner, ""))
	if text == "" {
		return spec
	}
	_ = json.Unmarshal([]byte(text), &spec)
	return spec
}

func approvalFrom(spec extSpec, mi *multiXML) flow.ApprovalSpec {
	var a flow.ApprovalSpec
	a.Collection = spec.Collection
	a.ElementVar = spec.ElementVar
	switch strings.ToUpper(spec.Strategy) {
	case "ANY":
		a.Strategy = flow.StrategyAny
	default:
		a.Strategy = flow.StrategyAll
	}
	if mi != nil {
		a.Sequential = strings.EqualFold(strings.TrimSpace(mi.IsSequential), "true")
		switch strings.ToUpper(strings.TrimSpace(mi.CompletionCondition)) {
		case "ANY":
			a.Strategy = flow.StrategyAny
		case "ALL":
			a.Strategy = flow.StrategyAll
		}
	}
	switch strings.ToUpper(spec.RejectMode) {
	case "STAY":
		a.RejectMode = flow.RejectStay
	default:
		a.RejectMode = flow.RejectTerminate
	}
	return a
}

// stripUEL 把 `${expr}` 包裹还原为裸表达式；已是裸表达式则原样返回。
func stripUEL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		s = s[2 : len(s)-1]
	}
	return strings.TrimSpace(s)
}

// splitCalledElement 子流程引用："key"、"key:3" 或 "key:v3"。
func splitCalledElement(ref string) (string, int) {
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		vStr := strings.TrimPrefix(ref[i+1:], "v")
		if v, err := strconv.Atoi(vStr); err == nil {
			return ref[:i], v
		}
	}
	return ref, 0
}

func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// ============================================================
// 导出
// ============================================================

// Write Definition → BPMN 2.0 XML（支持子集）。可被 bpmn-js / Camunda Modeler 打开。
// 若 def.Source 为含 DI 图形的 BPMN，导出回填图形与原命名空间声明（视觉无损）。
func Write(def *flow.Definition) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	open := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"` +
		` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` +
		` xmlns:flow="` + NsFlow + `" targetNamespace="` + NsFlow + `">`
	defsTag, diBlock := harvestSourceExtras(def.Source)
	closeTag := "</definitions>"
	if defsTag != "" {
		open = defsTag
		// 子元素无前缀：缺默认命名空间时注入，使其归入 BPMN 命名空间。
		if !strings.Contains(open, `xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"`) {
			open = strings.Replace(open, ">",
				` xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`, 1)
		}
		if !strings.Contains(open, "xmlns:xsi") {
			open = strings.Replace(open, ">",
				` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`, 1)
		}
		if i := strings.Index(open, "<"); i >= 0 {
			if j := strings.Index(open[i:], ":"); j >= 0 && j < strings.Index(open[i:], "definitions") {
				closeTag = "</" + open[i+1:i+j] + ":definitions>"
			}
		}
	}
	b.WriteString(open + "\n")
	b.WriteString(fmt.Sprintf("  <process id=%q name=%q isExecutable=\"true\">\n", def.Key, def.Name))

	ids := make([]string, 0, len(def.Nodes))
	for id := range def.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// 事件与任务。
	for _, id := range ids {
		n := def.Nodes[id]
		render := func() string {
			var nb strings.Builder
			b := &nb
			switch n.Type {
			case flow.NodeStartEvent:
				fmt.Fprintf(b, "    <startEvent id=%q name=%q/>\n", n.ID, n.Name)
			case flow.NodeEndEvent:
				fmt.Fprintf(b, "    <endEvent id=%q name=%q/>\n", n.ID, n.Name)
			case flow.NodeErrorEndEvent:
				fmt.Fprintf(b, "    <endEvent id=%q name=%q>\n", n.ID, n.Name)
				fmt.Fprintf(b, "      <errorEventDefinition errorRef=%q/>\n", n.ErrorCode)
				fmt.Fprintf(b, "    </endEvent>\n")
			case flow.NodeUserTask:
				b.WriteString("    <userTask id=" + quote(n.ID) + " name=" + quote(n.Name) + ">\n")
				b.WriteString("      <extensionElements>\n")
				spec := map[string]any{}
				if n.Approver != nil {
					spec["approvers"] = n.Approver.Rules
				}
				if n.Approval.Strategy == flow.StrategyAny {
					spec["strategy"] = "ANY"
				}
				if n.Approval.RejectMode == flow.RejectStay {
					spec["rejectMode"] = "STAY"
				}
				if n.Timer != nil {
					spec["due"] = n.Timer.Duration.Std().String()
					if n.Timer.Action != flow.TimerActionNone {
						spec["action"] = string(n.Timer.Action)
					}
				}
				payload, _ := json.Marshal(spec)
				fmt.Fprintf(b, "        <flow:assignees>%s</flow:assignees>\n", string(payload))
				b.WriteString("      </extensionElements>\n")
				b.WriteString("    </userTask>\n")
			case flow.NodeScriptTask, flow.NodeServiceTask:
				tag := "scriptTask"
				if n.Type == flow.NodeServiceTask {
					tag = "serviceTask"
				}
				lang, scope := "", ""
				if n.Script != nil {
					lang = n.Script.Lang
					if n.Script.Scope == "local" {
						scope = ` flow:scope="local"`
					}
				}
				fmt.Fprintf(b, "    <%s id=%q name=%q scriptFormat=%q%s>\n", tag, n.ID, n.Name, lang, scope)
				if n.Script != nil {
					fmt.Fprintf(b, "      <script><![CDATA[%s]]></script>\n", n.Script.Code)
				}
				fmt.Fprintf(b, "    </%s>\n", tag)
			case flow.NodeExclusiveGateway:
				if n.DefaultEdge != "" {
					fmt.Fprintf(b, "    <exclusiveGateway id=%q name=%q default=%q/>\n", n.ID, n.Name, n.DefaultEdge)
				} else {
					fmt.Fprintf(b, "    <exclusiveGateway id=%q name=%q/>\n", n.ID, n.Name)
				}
			case flow.NodeParallelGateway:
				fmt.Fprintf(b, "    <parallelGateway id=%q name=%q/>\n", n.ID, n.Name)
			case flow.NodeInclusiveGateway:
				fmt.Fprintf(b, "    <inclusiveGateway id=%q name=%q/>\n", n.ID, n.Name)
		case flow.NodeTimerEvent:
			dur := ""
			if n.Timer != nil {
				dur = WriteISODuration(n.Timer.Duration.Std())
			}
			fmt.Fprintf(b, "    <intermediateCatchEvent id=%q name=%q>\n", n.ID, n.Name)
			if n.Documentation != "" {
				fmt.Fprintf(b, "      <documentation>%s</documentation>\n", escDoc(n.Documentation))
			}
			fmt.Fprintf(b, "      <timerEventDefinition>\n")
			fmt.Fprintf(b, "        <timeDuration>%s</timeDuration>\n", dur)
			fmt.Fprintf(b, "      </timerEventDefinition>\n")
			fmt.Fprintf(b, "    </intermediateCatchEvent>\n")
		case flow.NodeMessageEvent:
			msg := ""
			if n.Message != "" {
				msg = n.Message
			}
			fmt.Fprintf(b, "    <intermediateCatchEvent id=%q name=%q>\n", n.ID, n.Name)
			if n.Documentation != "" {
				fmt.Fprintf(b, "      <documentation>%s</documentation>\n", escDoc(n.Documentation))
			}
			fmt.Fprintf(b, "      <messageEventDefinition messageRef=%q/>\n", msg)
			fmt.Fprintf(b, "    </intermediateCatchEvent>\n")
			case flow.NodeBoundaryEvent:
				dur := ""
				if n.Timer != nil {
					dur = WriteISODuration(n.Timer.Duration.Std())
				}
				cancel := "true"
				if !n.IsInterrupting() {
					cancel = "false"
				}
				fmt.Fprintf(b, "    <boundaryEvent id=%q name=%q attachedToRef=%q cancelActivity=%q>\n",
					n.ID, n.Name, n.AttachedTo, cancel)
				if n.ErrorCode != "" {
					fmt.Fprintf(b, "      <errorEventDefinition errorRef=%q/>\n", n.ErrorCode)
				} else {
					fmt.Fprintf(b, "      <timerEventDefinition>\n")
					fmt.Fprintf(b, "        <timeDuration>%s</timeDuration>\n", dur)
					fmt.Fprintf(b, "      </timerEventDefinition>\n")
				}
				fmt.Fprintf(b, "    </boundaryEvent>\n")
			case flow.NodeCallActivity:
				ref := n.SubprocessKey
				if n.SubprocessVersion > 0 {
					ref = fmt.Sprintf("%s:v%d", n.SubprocessKey, n.SubprocessVersion)
				}
				fmt.Fprintf(b, "    <callActivity id=%q name=%q calledElement=%q/>\n", n.ID, n.Name, ref)
			}
			return nb.String()
		}
		body := render()
		if n.Documentation != "" {
			// BPMN 约定 documentation 为首子元素；自闭合元素展开为开闭标签。
			docEl := "      <documentation>" + escDoc(n.Documentation) + "</documentation>\n"
			trimmed := strings.TrimRight(body, " \n")
			if strings.HasSuffix(trimmed, "/>") {
				body = strings.TrimSuffix(trimmed, "/>") + ">\n" + docEl + "    </" + string(n.Type) + ">\n"
			} else if i := strings.Index(body, ">\n"); i >= 0 {
				body = body[:i+2] + docEl + body[i+2:]
			}
		}
		b.WriteString(body)
	}

	for _, e := range def.Edges {
		if e.Condition != "" {
			fmt.Fprintf(&b, "    <sequenceFlow id=%q name=%q sourceRef=%q targetRef=%q>"+
				"<conditionExpression xsi:type=\"tFormalExpression\">${%s}</conditionExpression></sequenceFlow>\n",
				e.ID, e.Name, e.From, e.To, e.Condition)
		} else {
			fmt.Fprintf(&b, "    <sequenceFlow id=%q name=%q sourceRef=%q targetRef=%q/>\n",
				e.ID, e.Name, e.From, e.To)
		}
	}

	b.WriteString("  </process>\n")
	if diBlock != "" {
		b.WriteString("  " + diBlock + "\n")
	}
	b.WriteString(closeTag + "\n")
	return []byte(b.String()), nil
}

func quote(s string) string {
	return strconv.Quote(s)
}

// escDoc XML 文本转义（documentation 内容）。
func escDoc(s string) string {
	var bb bytes.Buffer
	_ = xml.EscapeText(&bb, []byte(s))
	return bb.String()
}

var defsTagRe = regexp.MustCompile(`(?is)<(?:[A-Za-z0-9_.-]+:)?definitions[^>]*>`)
var defsCloseRe = regexp.MustCompile(`(?is)</(?:[A-Za-z0-9_.-]+:)?definitions>`)

// harvestSourceExtras 从原始 BPMN 源中回收图形信息（BPMNDiagram/DI）与
// definitions 开标签（保留全部命名空间声明），供导出回填——往返对
// bpmn-js / Camunda Modeler 视觉无损。兼容带前缀（bpmn:definitions）与
// 无前缀（definitions）两种根元素形态。
func harvestSourceExtras(source []byte) (defsTag, diBlock string) {
	if len(source) == 0 {
		return "", ""
	}
	src := string(source)
	hasDI := strings.Contains(src, ":BPMNDiagram") || strings.Contains(src, "<BPMNDiagram")
	if !hasDI {
		return "", ""
	}
	if m := defsTagRe.FindString(src); m != "" {
		defsTag = m
	}
	start := strings.Index(src, ":BPMNDiagram")
	if start >= 0 {
		start = strings.LastIndex(src[:start], "<")
	}
	if start < 0 {
		start = strings.Index(src, "<BPMNDiagram")
	}
	closeLoc := defsCloseRe.FindStringIndex(src)
	if start >= 0 && closeLoc != nil && closeLoc[0] > start {
		diBlock = strings.TrimSpace(src[start:closeLoc[0]])
	}
	if diBlock == "" {
		defsTag = ""
	}
	return defsTag, diBlock
}
