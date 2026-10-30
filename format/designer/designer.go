// Package designer 定义 go-wind-ui 流程设计器与 go-wind-flow 之间的
// JSON 契约（designer-json-v1）：领域结构 + UI 元数据（节点坐标、边拐点）。
//
// 契约细节见仓库 docs/designer_contract.md。要点：
//
//   - 节点/边的业务字段与引擎 flow.Node / flow.Edge 一一对应（同一 json tag），
//     设计器保存的文档可直接 Deploy（Format = "designer-json-v1"，亦可留空——
//     内容嗅探对 '{' 开头且含 "version":1 的文档命中本格式）；
//   - UI 元数据（ui.*）由服务端原样保留在文档里，引擎忽略；
//   - 定义不可变：设计器每次"发布"= Deploy 新版本。
package designer

import (
	"encoding/json"
	"fmt"

	"github.com/tx7do/go-wind-flow/flow"
)

// FormatName 注册到引擎的格式标识。
const FormatName = "designer-json-v1"

// Version 当前契约版本。
const Version = 1

// Position 节点在画布上的坐标。
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// EdgeUI 连线的 UI 元数据。
type EdgeUI struct {
	// Waypoints 拐点序列（画布坐标，不含端点）。
	Waypoints [][2]float64 `json:"waypoints,omitempty"`
}

// Node 设计器节点：领域字段（内嵌 flow.Node，json tag 一致）+ UI 元数据。
type Node struct {
	flow.Node
	UI *Position `json:"ui,omitempty"`
}

// Edge 设计器连线：source/target 遵循设计器（bpmn-js）惯例，
// 解析时映射为领域的 From/To。
type Edge struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Name      string  `json:"name,omitempty"`
	Condition string  `json:"condition,omitempty"`
	IsDefault bool    `json:"isDefault,omitempty"`
	UI        *EdgeUI `json:"ui,omitempty"`
}

// domain 领域边。
func (e Edge) domain() *flow.Edge {
	return &flow.Edge{
		ID:        e.ID,
		From:      e.Source,
		To:        e.Target,
		Name:      e.Name,
		Condition: e.Condition,
		IsDefault: e.IsDefault,
	}
}

func edgeFromDomain(e *flow.Edge) Edge {
	return Edge{
		ID:        e.ID,
		Source:    e.From,
		Target:    e.To,
		Name:      e.Name,
		Condition: e.Condition,
		IsDefault: e.IsDefault,
	}
}

// Document 设计器文档。
type Document struct {
	Version int    `json:"version"`
	Key     string `json:"key"`
	Name    string `json:"name,omitempty"`
	Nodes   []Node `json:"nodes"`
	Edges   []Edge `json:"edges"`
}

// Importer 实现 flow.Importer。
type Importer struct{}

func (Importer) Format() string { return FormatName }

func (Importer) Import(data []byte) (*flow.Definition, error) { return Parse(data) }

// Exporter 实现 flow.Exporter。
type Exporter struct{}

func (Exporter) Format() string { return FormatName }

// Export 从 Definition 导出设计器文档。
// 注意：引擎内的 Definition 不携带 UI 元数据（ui 字段为 nil）——
// 宿主如需保坐标，应保存原始文档而非依赖导出。
func (Exporter) Export(def *flow.Definition) ([]byte, error) { return Write(def) }

// Parse 设计器文档 → Definition。
func Parse(data []byte) (*flow.Definition, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: parse designer json: %v", flow.ErrInvalidDefinition, err)
	}
	if doc.Version != Version {
		return nil, fmt.Errorf("%w: designer json version %d, want %d",
			flow.ErrInvalidDefinition, doc.Version, Version)
	}
	if doc.Key == "" {
		return nil, fmt.Errorf("%w: designer json missing key", flow.ErrInvalidDefinition)
	}

	def := &flow.Definition{
		Key:   doc.Key,
		Name:  doc.Name,
		Nodes: make(map[string]*flow.Node, len(doc.Nodes)),
		Edges: make([]*flow.Edge, 0, len(doc.Edges)),
	}
	for i := range doc.Nodes {
		n := doc.Nodes[i].Node
		if n.ID == "" || n.Type == "" {
			return nil, fmt.Errorf("%w: node %d missing id or type", flow.ErrInvalidDefinition, i)
		}
		if _, dup := def.Nodes[n.ID]; dup {
			return nil, fmt.Errorf("%w: duplicate node id %s", flow.ErrInvalidDefinition, n.ID)
		}
		def.Nodes[n.ID] = &n
	}
	for i := range doc.Edges {
		de := doc.Edges[i]
		id := de.ID
		if id == "" {
			id = fmt.Sprintf("e_%d", i+1)
		}
		if def.Nodes[de.Source] == nil || def.Nodes[de.Target] == nil {
			return nil, fmt.Errorf("%w: edge %s references unknown node", flow.ErrInvalidDefinition, id)
		}
		edge := de.domain()
		edge.ID = id
		def.Edges = append(def.Edges, edge)
	}
	return def, nil
}

// Write Definition → 设计器文档。
func Write(def *flow.Definition) ([]byte, error) {
	doc := Document{
		Version: Version,
		Key:     def.Key,
		Name:    def.Name,
		Nodes:   make([]Node, 0, len(def.Nodes)),
		Edges:   make([]Edge, 0, len(def.Edges)),
	}
	for id, n := range def.Nodes {
		node := *n
		if node.ID == "" {
			node.ID = id
		}
		doc.Nodes = append(doc.Nodes, Node{Node: node})
	}
	// 稳定输出：按 ID 排序（map 遍历无序，保证导出可 diff）。
	sortNodes(doc.Nodes)
	for _, e := range def.Edges {
		doc.Edges = append(doc.Edges, edgeFromDomain(e))
	}
	sortEdges(doc.Edges)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func sortNodes(nodes []Node) {
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0 && nodes[j].ID < nodes[j-1].ID; j-- {
			nodes[j], nodes[j-1] = nodes[j-1], nodes[j]
		}
	}
}

func sortEdges(edges []Edge) {
	for i := 1; i < len(edges); i++ {
		for j := i; j > 0 && edges[j].ID < edges[j-1].ID; j-- {
			edges[j], edges[j-1] = edges[j-1], edges[j]
		}
	}
}
