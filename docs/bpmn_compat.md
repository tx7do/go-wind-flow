# BPMN 兼容策略（BPMN Compatibility）参考文档

> **定位**：本仓与 BPMN 2.0 生态互通的唯一权威说明——元素支持矩阵、
> `<flow:assignees>` 办理人扩展约定、UEL 条件映射、bpmn-js / Camunda Modeler
> 互通路径与格式插件机制。接存量 BPMN 文件、接流程设计器、或新增
> Importer/Exporter 之前先读它。

目标：**无缝吸引 BPMN 用户**——存量 BPMN 文件可直接部署，go-wind-flow 的定义
可导出为 BPMN 交由主流建模工具打开，BPMN 生态的建模习惯（元素命名、UEL 条件、
多实例语义）在引擎内一一对应。

## 1. 接口设计

BPMN 兼容通过引擎的两个正式接口落地（`flow/importer.go`）：

```go
type Importer interface {
	Format() string                       // "bpmn-xml"
	Import(data []byte) (*Definition, error)
}
type Exporter interface {
	Format() string
	Export(def *Definition) ([]byte, error)
}
```

装配方式（格式可插拔，宿主亦可注册自有格式）：

```go
eng := flow.New(
	flow.WithImporters(bpmn.Importer{}),
	flow.WithExporters(bpmn.Exporter{}),
)

// Format 留空时按内容嗅探：'<' 开头 → bpmn-xml，否则 → wind-json-v2
def, _ := eng.Deploy(ctx, flow.DeployRequest{Key: "leave", Data: bpmnBytes})

// 导出给 Camunda Modeler / bpmn-js
xmlBytes, _ := eng.ExportDefinition(ctx, "t1", "leave", 0, "bpmn-xml")
```

## 2. 支持矩阵（M0）

| BPMN 元素 | 支持 | 说明 |
|---|---|---|
| `startEvent` / `endEvent` | ✅ | 多终点支持 |
| `userTask` | ✅ | 办理人见 §3 |
| `scriptTask` / `serviceTask` | ✅ | `scriptFormat` + `<script>`，经 ScriptRunner 执行 |
| `exclusiveGateway` | ✅ | `default` 属性 → 默认流；条件为 UEL/裸表达式 |
| `parallelGateway` | ✅ | 分叉/汇合同元素按出入度区分（BPMN 标准做法） |
| `inclusiveGateway` | ✅ | 条件多分支 + 期望计数汇合 |
| `callActivity` | ✅ | `calledElement` 支持 `key` / `key:vN` |
| `multiInstanceLoopCharacteristics` | ✅ | 非顺序多实例；完成语义经 `completionCondition`（ALL/ANY）或扩展 JSON |
| `conditionExpression` | ✅ | `${...}` 包裹自动剥壳为裸表达式 |
| `intermediateCatchEvent` + `timerEventDefinition` | ✅ | M1 定时器节点；`timeDuration` 支持 ISO8601 天/时/分/秒（年/月/周日历单位显式拒绝） |
| `boundaryEvent` + `timerEventDefinition` | ✅ | M2 边界定时事件（挂接 userTask）：中断型（缺省）到期取消原任务并接管流程，`cancelActivity=false` 非中断型只派生升级分支（v1 一次性触发）；由 `ResumeDueTimers` 驱动 |
| `documentation` | ⛔ M2 | 透传到 Node.Documentation |
| `boundaryEvent` 消息/信号触发 / 消息与信号事件 / 顺序中间事件 | ⛔ M2 | 定时器触发已落地（见上一行） |
| 顺序多实例（isSequential=true） | ⛔ M2 | |
| lane / 泳道 | ⛔ | 展示层概念，不入引擎 |
| BPMN DI（图形坐标） | 透传保留 | 不解析不使用，导出不还原（M2 考虑透传） |

## 3. 办理人扩展约定

BPMN 标准没有"会签/角色解析/委托"语义，用扩展元素携带，解析顺序：

1. **`<flow:assignees>` 扩展元素**（JSON，全能力）：

```xml
<userTask id="leader_review" name="直属领导审批">
  <extensionElements>
    <flow:assignees>{
      "approvers": [{"kind": "LEADER"}, {"kind": "ROLE", "value": "HRBP"}],
      "strategy": "ALL",
      "rejectMode": "TERMINATE",
      "due": "24h"
    }</flow:assignees>
  </extensionElements>
</userTask>
```

`kind` 取值与引擎 `ApproverKind` 一致：`USER` / `STARTER` / `LEADER` / `POSITION` / `ROLE` / `EXPR`。

2. **标准/Activiti 风格属性兜底**：`assignee="u1"`、`candidateUsers="u1,u2"` → USER 规则。

3. **多实例完成语义**：`<completionCondition>ALL|ANY</completionCondition>` 与扩展 JSON 的 `strategy` 等效（扩展优先级低，二者取一即可）。

## 4. 条件表达式映射

| 来源 | 写法 | 引擎收到 |
|---|---|---|
| BPMN XML | `${days > 3 && urgent}` | `days > 3 && urgent`（剥壳） |
| wind-json | `"condition": "amount > 1000"` | 原样（已是裸表达式） |
| `"condition": "default"` | wind-json 默认流标记 | `IsDefault = true` |

求值器可插拔：内置 `SimpleEvaluator` 覆盖比较/逻辑/包含运算（零依赖、确定性、
禁函数调用）；生产可换 `contrib/exprlang`（与 OA/IM 行为完全一致）或 go-scripts
的 CEL 引擎。**UEL 语法子集与三者兼容**——只要表达式落在三者交集内即可跨引擎复用。

## 5. 与主流工具互通

- **bpmn-js / bpmn.io**：导出的 XML 可直接打开编辑；`flow:assignees` 扩展元素
  在 bpmn-js 中原样保留（M2 提供配套 properties-panel 扩展）。
- **Camunda Modeler**：同上；条件表达式的 `${}` 习惯完全兼容。
- **Flowable/Activiti 导出的 BPMN**：`assignee`/`candidateUsers` 属性直接识别；
  UEL 中依赖 Java Bean 方法的表达式（如 `${userService.isVip(execution)}`）不支持，
  需改写为变量表达式或脚本任务。

## 6. 非 BPMN 格式

`Importer` 是公开接口——钉钉风格审批流 JSON、宜搭表单流等自有格式可实现
`Importer` 注册进引擎，与 BPMN 并存。`format/windjson` 就是第一个范例
（兼容 go-wind-oa/im 存量 node_config）。
