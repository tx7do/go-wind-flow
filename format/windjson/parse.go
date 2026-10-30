// Package windjson 提供 go-wind-oa / go-wind-im 现有 node_config JSON
// （version:2 图格式与更早的节点数组旧格式）到 flow.Definition 的转换，
// 保证存量流程定义可无缝迁移到 go-wind-flow。
package windjson

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tx7do/go-wind-flow/flow"
)

// FormatName 注册到引擎的格式标识。
const FormatName = "wind-json-v2"

// Importer 实现 flow.Importer。
type Importer struct{}

func (Importer) Format() string { return FormatName }

func (Importer) Import(data []byte) (*flow.Definition, error) { return Parse(data) }

// ============================================================
// OA/IM node_config JSON 结构（兼容层，字段与 go-wind-im 现实现一一对应）
// ============================================================

type graphJSON struct {
	Version int          `json:"version"`
	Nodes   []nodeJSON   `json:"nodes"`
	Edges   []edgeJSON   `json:"edges"`
}

type nodeJSON struct {
	ID                   string         `json:"id"`
	Type                 string         `json:"type"`
	Approvers            []approverJSON `json:"approvers,omitempty"`
	Strategy             string         `json:"strategy,omitempty"`
	SubprocessDefinition string         `json:"subprocessDefinition,omitempty"`
	// 旧格式字段（节点数组路径用）
	ApproverType string `json:"approver_type,omitempty"`
	Approver     uint32 `json:"approver,omitempty"`
}

type approverJSON struct {
	Type string `json:"type"`
	ID   uint32 `json:"id,omitempty"`
	Code string `json:"code,omitempty"`
}

type edgeJSON struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
}

// OA/IM 节点类型常量。
const (
	imStart               = "START"
	imEnd                 = "END"
	imTask                = "TASK"
	imExclusiveGateway    = "EXCLUSIVE_GATEWAY"
	imParallelGatewayFork = "PARALLEL_GATEWAY_FORK"
	imParallelGatewayJoin = "PARALLEL_GATEWAY_JOIN"
	imSubprocess          = "SUBPROCESS"

	conditionDefault = "default"
)

// Parse node_config JSON → Definition。同时兼容 v2 图格式与旧版节点数组。
func Parse(data []byte) (*flow.Definition, error) {
	var graph graphJSON
	if err := json.Unmarshal(data, &graph); err == nil && graph.Version == 2 {
		return fromGraph(&graph)
	}

	var legacy []nodeJSON
	if err := json.Unmarshal(data, &legacy); err == nil && len(legacy) > 0 {
		return fromLegacy(legacy)
	}
	return nil, fmt.Errorf("%w: invalid node config json", flow.ErrInvalidDefinition)
}

func fromGraph(g *graphJSON) (*flow.Definition, error) {
	def := &flow.Definition{Nodes: map[string]*flow.Node{}, Edges: []*flow.Edge{}}
	for _, nj := range g.Nodes {
		if nj.ID == "" || nj.Type == "" {
			return nil, fmt.Errorf("%w: node missing id or type", flow.ErrInvalidDefinition)
		}
		node := &flow.Node{ID: nj.ID}
		switch nj.Type {
		case imStart:
			node.Type = flow.NodeStartEvent
		case imEnd:
			node.Type = flow.NodeEndEvent
		case imTask:
			node.Type = flow.NodeUserTask
			node.Name = nj.ID
			rules := make([]flow.ApproverRule, 0, len(nj.Approvers))
			for _, a := range nj.Approvers {
				r := approverRule(a)
				if r.Kind != "" {
					rules = append(rules, r)
				}
			}
			node.Approver = &flow.ApproverSpec{Rules: rules}
			if strings.EqualFold(nj.Strategy, "ANY") {
				node.Approval.Strategy = flow.StrategyAny
			} else {
				node.Approval.Strategy = flow.StrategyAll
			}
			node.Approval.RejectMode = flow.RejectTerminate
		case imExclusiveGateway:
			node.Type = flow.NodeExclusiveGateway
		case imParallelGatewayFork, imParallelGatewayJoin:
			node.Type = flow.NodeParallelGateway
		case imSubprocess:
			node.Type = flow.NodeCallActivity
			node.SubprocessKey, node.SubprocessVersion = splitRef(nj.SubprocessDefinition)
		default:
			return nil, fmt.Errorf("%w: unknown node type %q (%s)", flow.ErrInvalidDefinition, nj.Type, nj.ID)
		}
		def.Nodes[nj.ID] = node
	}

	for i, ej := range g.Edges {
		edge := &flow.Edge{
			ID:   fmt.Sprintf("e_%d", i+1),
			From: ej.From,
			To:   ej.To,
		}
		if ej.Condition == conditionDefault {
			edge.IsDefault = true
		} else {
			edge.Condition = ej.Condition
		}
		if def.Nodes[ej.From] == nil || def.Nodes[ej.To] == nil {
			return nil, fmt.Errorf("%w: edge %d references unknown node", flow.ErrInvalidDefinition, i+1)
		}
		def.Edges = append(def.Edges, edge)
	}
	return def, nil
}

// fromLegacy 旧格式节点数组 → 等价线性图（与 go-wind-im buildGraphFromLegacy 对齐）。
func fromLegacy(nodes []nodeJSON) (*flow.Definition, error) {
	def := &flow.Definition{Nodes: map[string]*flow.Node{}, Edges: []*flow.Edge{}}

	def.Nodes["__legacy_start"] = &flow.Node{ID: "__legacy_start", Type: flow.NodeStartEvent}
	def.Nodes["__legacy_end"] = &flow.Node{ID: "__legacy_end", Type: flow.NodeEndEvent}

	prev := "__legacy_start"
	for i, ln := range nodes {
		id := fmt.Sprintf("__legacy_node_%d", i)
		node := &flow.Node{ID: id, Type: flow.NodeUserTask, Name: ln.ID}
		rules := make([]flow.ApproverRule, 0, len(ln.Approvers))
		for _, a := range ln.Approvers {
			r := approverRule(a)
			if r.Kind != "" {
				rules = append(rules, r)
			}
		}
		if len(rules) == 0 && ln.ApproverType == "USER" && ln.Approver != 0 {
			rules = append(rules, flow.ApproverRule{
				Kind:  flow.ApproverUser,
				Value: strconv.FormatUint(uint64(ln.Approver), 10),
			})
		}
		node.Approver = &flow.ApproverSpec{Rules: rules}
		if strings.EqualFold(ln.Strategy, "ANY") {
			node.Approval.Strategy = flow.StrategyAny
		} else {
			node.Approval.Strategy = flow.StrategyAll
		}
		node.Approval.RejectMode = flow.RejectTerminate
		def.Nodes[id] = node

		def.Edges = append(def.Edges, &flow.Edge{
			ID: fmt.Sprintf("e_%d", len(def.Edges)+1), From: prev, To: id,
		})
		prev = id
	}
	def.Edges = append(def.Edges, &flow.Edge{
		ID: fmt.Sprintf("e_%d", len(def.Edges)+1), From: prev, To: "__legacy_end",
	})
	return def, nil
}

func approverRule(a approverJSON) flow.ApproverRule {
	switch strings.ToUpper(a.Type) {
	case "USER":
		return flow.ApproverRule{Kind: flow.ApproverUser, Value: strconv.FormatUint(uint64(a.ID), 10)}
	case "LEADER":
		return flow.ApproverRule{Kind: flow.ApproverLeader}
	case "POSITION":
		return flow.ApproverRule{Kind: flow.ApproverPosition, Value: a.Code}
	case "ROLE":
		return flow.ApproverRule{Kind: flow.ApproverRole, Value: a.Code}
	}
	return flow.ApproverRule{}
}

func splitRef(ref string) (string, int) {
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		vStr := strings.TrimPrefix(ref[i+1:], "v")
		if v, err := strconv.Atoi(vStr); err == nil {
			return ref[:i], v
		}
	}
	return ref, 0
}
