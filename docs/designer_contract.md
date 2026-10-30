# 设计器契约（Designer Contract）参考文档

> **定位**：go-wind-ui 流程设计器与 go-wind-flow 之间 JSON 契约（designer-json-v1）
> 的唯一权威说明——文档结构、字段语义、设计器↔服务端交互协议与演进规则。
> 设计器开发、BFF 保存/发布接口、或契约升版之前先读它。

## 1. 设计原则

1. **业务字段即领域字段**：节点/边的业务语义与引擎 `flow.Node / flow.Edge` 一一对应
   （同一 JSON tag），设计器保存的文档**不做转换直接 Deploy**；
2. **UI 元数据服务端不解释**：`ui.*`（坐标、拐点）由服务端原样保留，引擎忽略；
   宿主如需保坐标，应保存**原始文档**而非依赖引擎导出（引擎导出不还原 UI）；
3. **发布即版本化**：设计器"发布"= 携带文档调 Deploy，引擎校验并生成新版本，
   在途实例不受影响。

## 2. 文档结构（designer-json-v1）

```json
{
  "version": 1,
  "key": "leave",
  "name": "请假审批",
  "nodes": [
    { "id": "start",  "type": "startEvent", "name": "提交", "ui": { "x": 100, "y": 200 } },
    { "id": "leader", "type": "userTask", "name": "直属领导审批",
      "approver": { "rules": [ { "kind": "LEADER" } ], "strategy": "ALL",
                    "rejectMode": "TERMINATE" },
      "timer": { "duration": "24h" },
      "ui": { "x": 300, "y": 200 } },
    { "id": "gw", "type": "exclusiveGateway", "name": "是否超过3天",
      "defaultEdge": "f3", "ui": { "x": 500, "y": 200 } },
    { "id": "end", "type": "endEvent", "ui": { "x": 900, "y": 200 } }
  ],
  "edges": [
    { "id": "f1", "source": "start", "target": "leader",
      "ui": { "waypoints": [[200, 200]] } },
    { "id": "f3", "source": "gw", "target": "end", "isDefault": true },
    { "id": "f4", "source": "gw", "target": "director",
      "condition": "days > 3" }
  ]
}
```

### 字段语义

| 字段 | 语义 | 校验 |
|---|---|---|
| `version` | 契约版本，当前恒为 `1` | 不匹配即拒绝 |
| `key` | 定义标识（发布后不可改） | 必填 |
| `nodes[].type` | 与引擎 `NodeType` 一致：`startEvent` / `endEvent` / `userTask` / `scriptTask` / `serviceTask` / `intermediateTimerEvent` / `exclusiveGateway` / `parallelGateway` / `inclusiveGateway` / `callActivity` | 未知类型拒绝 |
| `nodes[].approver` | 办理人规格：`rules[].kind ∈ USER/STARTER/LEADER/POSITION/ROLE/EXPR`、`strategy: ALL/ANY`、`rejectMode: TERMINATE/STAY` | 部署时校验 |
| `nodes[].timer.duration` | 任务超时（Go duration，如 `24h`） | — |
| `nodes[].script` | `{lang, code}`，lang 由 ScriptRunner 约定 | 部署时校验 |
| `nodes[].subprocessKey` / `subprocessVersion` | 子流程引用 | 部署时校验 |
| `edges[].source/target` | 端点节点 ID（设计器惯例命名，服务端映射为 From/To） | 引用存在性 |
| `edges[].condition` | 裸布尔表达式（排他/包容网关出边） | 求值 fail-closed |
| `edges[].isDefault` | 默认流 | 排他网关恰一条 |
| `nodes[].ui` / `edges[].ui` | 坐标 / 拐点 | 服务端不校验 |

完整校验规则（拓扑、可达性、网关约束）见 [architecture_design.md §validate](./architecture_design.md)
与引擎 `flow.ValidateDefinition`——**契约不重复定义校验，发布时引擎是唯一裁判**。

## 3. 设计器 ↔ 服务端交互协议

设计器不直连引擎，全部经宿主 BFF；BFF 转调 go-wind-flow 的 gRPC 服务（`api` 子模块）。

| 设计器动作 | BFF 接口（建议） | 后端调用 |
|---|---|---|
| 画布保存（草稿） | `PUT /designer/flows/{key}/draft` | 宿主自存原始文档（对象存储/表），**不 Deploy** |
| 校验（发布前试算） | `POST /designer/flows/{key}/validate` | `DeployDefinition` 到测试租户（或后续增加 DryRun RPC） |
| 发布 | `POST /designer/flows/{key}/publish` | `DeployDefinition(format="designer-json-v1")`，文档原文与版本号一并落库 |
| 版本列表 / 详情 | `GET /designer/flows/{key}/versions` | `ListDefinitions` / `GetDefinition`（`source` 即当时文档） |
| 导出 BPMN | `GET /designer/flows/{key}/versions/{v}?format=bpmn-xml` | `ExportDefinition`（给 Camunda Modeler / bpmn-js 离线打开） |
| 运行态（待办/实例/历史） | 各宿主自有页面 | `ListPendingTasks` / `ListInstances` / `ListInstanceHistory` 等 |

交互约定：

- **草稿与发布分离**：草稿是设计器私有的工作副本（可不受部署校验约束）；
  发布才进引擎的不可变版本序列；
- **错误回传**：Deploy 的校验错误（`InvalidArgument`）应原样透出给设计器，
  引擎错误信息带节点 ID，设计器据此定位画布元素；
- **坐标保鲜**：发布请求携带完整文档（含 `ui`），BFF 在 Deploy 成功后把原文档
  与 `definition.version` 关联存储；引擎 `Definition.Source` 亦保留原文（审计）。

## 4. 演进规则

- **只加不改**：新增节点类型 / 字段一律向后兼容（旧文档仍可解析）；
- **升版**：出现不兼容变更时 `version` 升至 2，`format/designer` 同时保留
  新旧解析（Importer 按 version 分派），并在本文档记录迁移说明；
- **单一裁判**：字段的运行时语义以引擎代码为准，本文档描述契约形态；
  两边不一致时改本文档（教程层规则同家族约定）。

## 5. 与既有格式的关系

| 格式 | 定位 |
|---|---|
| `designer-json-v1`（本契约） | 设计器读写；领域字段 + UI 元数据 |
| `wind-json-v2` | go-wind-oa/im 存量兼容（迁移用，不再扩展） |
| `bpmn-xml` | 与 BPMN 生态互通（导入/导出） |
