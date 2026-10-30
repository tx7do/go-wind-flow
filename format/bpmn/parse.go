// Package bpmn 提供 BPMN 2.0 XML 与 flow.Definition 的双向转换，
// 是 go-wind-flow 与 BPMN 生态（bpmn-js、Camunda Modeler 等）互通的桥梁。
//
// 支持子集与扩展约定见 docs/bpmn-compat.md。
package bpmn

import (
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

	StartEvents []eventXML      `xml:"startEvent"`
	EndEvents   []eventXML      `xml:"endEvent"`
	UserTasks   []userTaskXML   `xml:"userTask"`
	ScriptTasks []scriptTaskXML `xml:"scriptTask"`
	ServiceTasks []scriptTaskXML `xml:"serviceTask"`
	TimerEvents []catchEventXML `xml:"intermediateCatchEvent"`

	ExclusiveGateways []gatewayXML `xml:"exclusiveGateway"`
	ParallelGateways  []gatewayXML `xml:"parallelGateway"`
	InclusiveGateways []gatewayXML `xml:"inclusiveGateway"`

	CallActivities []callXML `xml:"callActivity"`

	SequenceFlows []seqFlowXML `xml:"sequenceFlow"`
}

type eventXML struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

type userTaskXML struct {
	ID             string    `xml:"id,attr"`
	Name           string    `xml:"name,attr"`
	Assignee       string    `xml:"assignee,attr"`
	CandidateUsers string    `xml:"candidateUsers,attr"`
	Extension      *extXML   `xml:"extensionElements"`
	MultiInstance  *multiXML `xml:"multiInstanceLoopCharacteristics"`
}

type extXML struct {
	Inner string `xml:",innerxml"`
}

type multiXML struct {
	IsSequential        string `xml:"isSequential,attr"`
	CompletionCondition string `xml:"completionCondition"`
}

type scriptTaskXML struct {
	ID           string         `xml:"id,attr"`
	Name         string         `xml:"name,attr"`
	ScriptFormat string         `xml:"scriptFormat,attr"`
	Script       *scriptBodyXML `xml:"script"`
}

type scriptBodyXML struct {
	Inner string `xml:",innerxml"`
}

// catchEventXML 中间捕获事件（当前仅支持定时器定义）。
type catchEventXML struct {
	ID    string       `xml:"id,attr"`
	Name  string       `xml:"name,attr"`
	Timer *timerDefXML `xml:"timerEventDefinition"`
}

type timerDefXML struct {
	Duration string `xml:"timeDuration"`
}

type gatewayXML struct {
	ID      string `xml:"id,attr"`
	Name    string `xml:"name,attr"`
	Default string `xml:"default,attr"`
}

type callXML struct {
	ID            string `xml:"id,attr"`
	Name          string `xml:"name,attr"`
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
		add(&flow.Node{ID: s.ID, Name: s.Name, Type: flow.NodeStartEvent})
	}
	for _, s := range p.EndEvents {
		add(&flow.Node{ID: s.ID, Name: s.Name, Type: flow.NodeEndEvent})
	}
	for _, t := range p.UserTasks {
		node := &flow.Node{ID: t.ID, Name: t.Name, Type: flow.NodeUserTask}
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
		node.Approver = &flow.ApproverSpec{Rules: rules}
		node.Approval = approvalFrom(spec, t.MultiInstance)
		if spec.Due != "" {
			if d, err := parseDuration(spec.Due); err == nil {
				node.Timer = &flow.TimerSpec{Duration: flow.Duration(d)}
			}
		}
		add(node)
	}
	for _, t := range p.ScriptTasks {
		node := &flow.Node{ID: t.ID, Name: t.Name, Type: flow.NodeScriptTask}
		if t.Script != nil {
			node.Script = &flow.ScriptSpec{Lang: t.ScriptFormat, Code: strings.TrimSpace(t.Script.Inner)}
		}
		add(node)
	}
	for _, t := range p.ServiceTasks {
		node := &flow.Node{ID: t.ID, Name: t.Name, Type: flow.NodeServiceTask}
		if t.Script != nil {
			node.Script = &flow.ScriptSpec{Lang: t.ScriptFormat, Code: strings.TrimSpace(t.Script.Inner)}
		}
		add(node)
	}
	for _, ev := range p.TimerEvents {
		node := &flow.Node{ID: ev.ID, Name: ev.Name, Type: flow.NodeTimerEvent}
		if ev.Timer == nil || strings.TrimSpace(ev.Timer.Duration) == "" {
			return nil, fmt.Errorf("%w: intermediateCatchEvent %s missing timerEventDefinition/timeDuration",
				flow.ErrInvalidDefinition, ev.ID)
		}
		d, err := ParseISODuration(ev.Timer.Duration)
		if err != nil {
			return nil, fmt.Errorf("intermediateCatchEvent %s: %w", ev.ID, err)
		}
		node.Timer = &flow.TimerSpec{Duration: flow.Duration(d)}
		add(node)
	}
	for _, g := range p.ExclusiveGateways {
		add(&flow.Node{ID: g.ID, Name: g.Name, Type: flow.NodeExclusiveGateway, DefaultEdge: g.Default})
	}
	for _, g := range p.ParallelGateways {
		add(&flow.Node{ID: g.ID, Name: g.Name, Type: flow.NodeParallelGateway})
	}
	for _, g := range p.InclusiveGateways {
		add(&flow.Node{ID: g.ID, Name: g.Name, Type: flow.NodeInclusiveGateway})
	}
	for _, c := range p.CallActivities {
		key, version := splitCalledElement(c.CalledElement)
		add(&flow.Node{
			ID: c.ID, Name: c.Name, Type: flow.NodeCallActivity,
			SubprocessKey: key, SubprocessVersion: version,
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
	switch strings.ToUpper(spec.Strategy) {
	case "ANY":
		a.Strategy = flow.StrategyAny
	default:
		a.Strategy = flow.StrategyAll
	}
	if mi != nil {
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
func Write(def *flow.Definition) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"` +
		` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` +
		` xmlns:flow="` + NsFlow + `" targetNamespace="` + NsFlow + `">` + "\n")
	b.WriteString(fmt.Sprintf("  <process id=%q name=%q isExecutable=\"true\">\n", def.Key, def.Name))

	ids := make([]string, 0, len(def.Nodes))
	for id := range def.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// 事件与任务。
	for _, id := range ids {
		n := def.Nodes[id]
		switch n.Type {
		case flow.NodeStartEvent:
			fmt.Fprintf(&b, "    <startEvent id=%q name=%q/>\n", n.ID, n.Name)
		case flow.NodeEndEvent:
			fmt.Fprintf(&b, "    <endEvent id=%q name=%q/>\n", n.ID, n.Name)
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
			}
			payload, _ := json.Marshal(spec)
			fmt.Fprintf(&b, "        <flow:assignees>%s</flow:assignees>\n", string(payload))
			b.WriteString("      </extensionElements>\n")
			b.WriteString("    </userTask>\n")
		case flow.NodeScriptTask, flow.NodeServiceTask:
			tag := "scriptTask"
			if n.Type == flow.NodeServiceTask {
				tag = "serviceTask"
			}
			lang := ""
			if n.Script != nil {
				lang = n.Script.Lang
			}
			fmt.Fprintf(&b, "    <%s id=%q name=%q scriptFormat=%q>\n", tag, n.ID, n.Name, lang)
			if n.Script != nil {
				fmt.Fprintf(&b, "      <script><![CDATA[%s]]></script>\n", n.Script.Code)
			}
			fmt.Fprintf(&b, "    </%s>\n", tag)
		case flow.NodeExclusiveGateway:
			if n.DefaultEdge != "" {
				fmt.Fprintf(&b, "    <exclusiveGateway id=%q name=%q default=%q/>\n", n.ID, n.Name, n.DefaultEdge)
			} else {
				fmt.Fprintf(&b, "    <exclusiveGateway id=%q name=%q/>\n", n.ID, n.Name)
			}
		case flow.NodeParallelGateway:
			fmt.Fprintf(&b, "    <parallelGateway id=%q name=%q/>\n", n.ID, n.Name)
		case flow.NodeInclusiveGateway:
			fmt.Fprintf(&b, "    <inclusiveGateway id=%q name=%q/>\n", n.ID, n.Name)
		case flow.NodeTimerEvent:
			dur := ""
			if n.Timer != nil {
				dur = WriteISODuration(n.Timer.Duration.Std())
			}
			fmt.Fprintf(&b, "    <intermediateCatchEvent id=%q name=%q>\n", n.ID, n.Name)
			fmt.Fprintf(&b, "      <timerEventDefinition>\n")
			fmt.Fprintf(&b, "        <timeDuration>%s</timeDuration>\n", dur)
			fmt.Fprintf(&b, "      </timerEventDefinition>\n")
			fmt.Fprintf(&b, "    </intermediateCatchEvent>\n")
		case flow.NodeCallActivity:
			ref := n.SubprocessKey
			if n.SubprocessVersion > 0 {
				ref = fmt.Sprintf("%s:v%d", n.SubprocessKey, n.SubprocessVersion)
			}
			fmt.Fprintf(&b, "    <callActivity id=%q name=%q calledElement=%q/>\n", n.ID, n.Name, ref)
		}
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

	b.WriteString("  </process>\n</definitions>\n")
	return []byte(b.String()), nil
}

func quote(s string) string {
	return strconv.Quote(s)
}
