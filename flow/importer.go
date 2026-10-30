package flow

import (
	"fmt"
	"sort"
	"sync"
)

// Importer 定义导入器：把一种序列化格式转换为引擎内部 Definition。
//
// 这是 BPMN 兼容的正式接口——format/bpmn（BPMN 2.0 XML）与 format/windjson
// （go-wind-oa/im 的 node_config v2 JSON）都实现本接口；宿主也可注册自有
// 格式（如钉钉风格审批流 JSON），对引擎完全透明。
type Importer interface {
	// Format 返回格式标识，如 "bpmn-xml"、"wind-json-v2"。
	Format() string
	// Import 解析并构建 Definition（不含 Key/TenantID/Version，由 Deploy 填充）。
	Import(data []byte) (*Definition, error)
}

// Exporter 定义导出器：把 Definition 序列化为目标格式。
// 典型用途：把设计器 JSON 导出为 BPMN 交由 bpmn-js / Camunda Modeler 打开。
type Exporter interface {
	Format() string
	Export(def *Definition) ([]byte, error)
}

// FormatRegistry 导入/导出器注册表。
type FormatRegistry struct {
	mu         sync.RWMutex
	importers  map[string]Importer
	exporters  map[string]Exporter
	detector   func(data []byte) string // 可选：按内容嗅探格式
	defaultFmt string
}

func NewFormatRegistry() *FormatRegistry {
	return &FormatRegistry{
		importers: map[string]Importer{},
		exporters: map[string]Exporter{},
	}
}

// RegisterImporter 注册导入器，同格式覆盖。
func (r *FormatRegistry) RegisterImporter(im Importer) *FormatRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.importers[im.Format()] = im
	return r
}

// RegisterExporter 注册导出器，同格式覆盖。
func (r *FormatRegistry) RegisterExporter(ex Exporter) *FormatRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exporters[ex.Format()] = ex
	return r
}

// SetDefault 设置 Format 为空时的缺省格式。
func (r *FormatRegistry) SetDefault(format string) *FormatRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultFmt = format
	return r
}

// SetDetector 设置内容嗅探器（Format 为空时先于 defaultFmt 生效）。
func (r *FormatRegistry) SetDetector(fn func(data []byte) string) *FormatRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.detector = fn
	return r
}

// ResolveImport 按 format 选择导入器；format 为空时依次尝试嗅探与缺省。
func (r *FormatRegistry) ResolveImport(format string, data []byte) (Importer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if format == "" {
		if r.detector != nil {
			format = r.detector(data)
		}
		if format == "" {
			format = r.defaultFmt
		}
	}
	im, ok := r.importers[format]
	if !ok {
		return nil, fmt.Errorf("no importer registered for format %q (have %v)", format, r.formats())
	}
	return im, nil
}

func (r *FormatRegistry) formats() []string {
	var fs []string
	for f := range r.importers {
		fs = append(fs, f)
	}
	sort.Strings(fs)
	return fs
}
