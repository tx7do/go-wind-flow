package bpmn

import (
	"fmt"
	"strings"

	"github.com/tx7do/go-wind-flow/flow"
)

// flattenSubProcess 嵌入子流程扁平化（v1：SES 单入单出约束）。
//
// 将 `<subProcess>` 内部元素以 `<subProcessID>.<childID>` 前缀并入父图：
//   - 内部 start/end 不落为节点（避免双起点/双终点），相关内部边丢弃，
//     入口/出口经外部顺序流重定向衔接；
//   - 内部恰好 1 个普通 start、1 个普通 end（error end 拒绝）；
//   - 边界事件挂接到 subProcess 本身、事件子流程、MI 子流程：显式拒绝。
//
// 限制（文档化于 docs/bpmn_compat.md）：扁平化后导出不还原嵌套结构；
// 多实例嵌入子流程不支持。
func flattenSubProcess(def *flow.Definition, sp subProcessXML) error {
	prefix := sp.ID + "."
	if strings.TrimSpace(sp.ID) == "" {
		return fmt.Errorf("%w: embedded subProcess missing id", flow.ErrInvalidDefinition)
	}
	if len(sp.StartEvents) != 1 {
		return fmt.Errorf("%w: embedded subProcess %s must have exactly 1 startEvent (flatten v1)",
			flow.ErrInvalidDefinition, sp.ID)
	}
	innerEnd := ""
	for _, e := range sp.EndEvents {
		if e.Error != nil {
			return fmt.Errorf("%w: embedded subProcess %s error end unsupported (flatten v1)",
				flow.ErrInvalidDefinition, sp.ID)
		}
		innerEnd = e.ID
	}
	if innerEnd == "" {
		return fmt.Errorf("%w: embedded subProcess %s must have exactly 1 endEvent (flatten v1)",
			flow.ErrInvalidDefinition, sp.ID)
	}
	// 外部已挂接边界到 subProcess 本身：拒绝。
	for id, n := range def.Nodes {
		if n.Type == flow.NodeBoundaryEvent && n.AttachedTo == sp.ID {
			return fmt.Errorf("%w: boundary %s attached to embedded subProcess %s unsupported (flatten v1)",
				flow.ErrInvalidDefinition, id, sp.ID)
		}
	}

	add := func(n *flow.Node) error {
		if _, dup := def.Nodes[n.ID]; dup {
			return fmt.Errorf("%w: duplicate node id %s after flatten", flow.ErrInvalidDefinition, n.ID)
		}
		def.Nodes[n.ID] = n
		return nil
	}

	userRules := func(t userTaskXML) []flow.ApproverRule {
		spec := parseExt(t.Extension)
		rules := spec.Approvers
		if len(rules) == 0 {
			if t.Assignee != "" {
				rules = append(rules, flow.ApproverRule{Kind: flow.ApproverUser, Value: t.Assignee})
			}
			for _, uid := range strings.Split(t.CandidateUsers, ",") {
				if uid = strings.TrimSpace(uid); uid != "" {
					rules = append(rules, flow.ApproverRule{Kind: flow.ApproverUser, Value: uid})
				}
			}
		}
		for _, g := range strings.Split(t.CandidateGroups, ",") {
			if g = strings.TrimSpace(g); g != "" {
				rules = append(rules, flow.ApproverRule{Kind: flow.ApproverGroup, Value: g})
			}
		}
		return rules
	}

	// 内部顺序流：引用加前缀；涉及内部 start/end 的边丢弃，
	// 同时定位 SES 的入口/出口节点（外部边重定向目标）。
	firstInner, lastInner := "", ""
	for _, f := range sp.SequenceFlows {
		from, to := prefix+f.SourceRef, prefix+f.TargetRef
		if f.SourceRef == sp.StartEvents[0].ID {
			firstInner = to
			continue
		}
		if f.TargetRef == sp.EndEvents[0].ID {
			lastInner = from
			continue
		}
		edge := &flow.Edge{ID: prefix + f.ID, From: from, To: to, Name: f.Name}
		if f.Condition != nil {
			edge.Condition = stripUEL(f.Condition.Text)
		}
		def.Edges = append(def.Edges, edge)
	}
	if firstInner == "" || lastInner == "" {
		return fmt.Errorf("%w: embedded subProcess %s missing inner entry/exit flow (SES)",
			flow.ErrInvalidDefinition, sp.ID)
	}

	for _, t := range sp.UserTasks {
		node := &flow.Node{
			ID:            prefix + t.ID,
			Name:          t.Name,
			Type:          flow.NodeUserTask,
			Documentation: t.Doc,
			Approver:      &flow.ApproverSpec{Rules: userRules(t)},
			Approval:      approvalFrom(parseExt(t.Extension), t.MultiInstance),
		}
		if spec := parseExt(t.Extension); spec.Due != "" {
			if d, err := parseDuration(spec.Due); err == nil {
				node.Timer = &flow.TimerSpec{Duration: flow.Duration(d), Action: flow.TimerAction(spec.Action)}
			}
		}
		if err := add(node); err != nil {
			return err
		}
	}
	for _, t := range append(sp.ScriptTasks, sp.ServiceTasks...) {
		node := &flow.Node{ID: prefix + t.ID, Name: t.Name, Type: flow.NodeScriptTask, Documentation: t.Doc}
		if t.Script != nil {
			node.Script = &flow.ScriptSpec{
				Lang: t.ScriptFormat, Code: strings.TrimSpace(t.Script.Inner), Scope: t.scopeAttr(),
			}
		}
		if err := add(node); err != nil {
			return err
		}
	}
	for _, g := range sp.ExclusiveGateways {
		if err := add(&flow.Node{
			ID: prefix + g.ID, Name: g.Name, Type: flow.NodeExclusiveGateway,
			DefaultEdge: prefixIf(g.Default, prefix), Documentation: g.Doc,
		}); err != nil {
			return err
		}
	}
	for _, g := range sp.ParallelGateways {
		if err := add(&flow.Node{ID: prefix + g.ID, Name: g.Name, Type: flow.NodeParallelGateway, Documentation: g.Doc}); err != nil {
			return err
		}
	}

	// 外部顺序流重定向：入边指向入口节点，出边来自出口节点。
	for _, e := range def.Edges {
		if strings.HasPrefix(e.From, prefix) || strings.HasPrefix(e.To, prefix) {
			continue // 内部流
		}
		if e.From == sp.ID {
			e.From = lastInner
		}
		if e.To == sp.ID {
			e.To = firstInner
		}
	}
	return nil
}

func prefixIf(v, prefix string) string {
	if v == "" {
		return ""
	}
	return prefix + v
}
