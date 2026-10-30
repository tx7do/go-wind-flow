<div align="center">

# go-wind-flow

**企业级轻量流程引擎 · go-wind 家族的工作流底座**

持久化驱动的 Token 状态机 · BPMN 2.0 兼容 · 零核心依赖 · 单二进制友好

</div>

---

`go-wind-flow` 是 [go-wind](https://github.com/tx7do/go-wind) 生态的流程引擎，从
go-wind-oa / go-wind-im 的审批流引擎中提取、泛化而来，并与 [go-scripts](https://github.com/tx7do/go-scripts)
深度配合：**引擎管流转，脚本管"流转之外的所有逻辑"**。

对标 Java 世界 Flowable / Camunda / Activiti 的常用子集，为 Go 私有部署市场
（OA、ERP、CRM、低代码平台）提供审批流/业务流底座。

## 特性

- **BPMN 2.0 兼容**：`format/bpmn` 导入导出 BPMN XML，可直接对接 bpmn-js、Camunda Modeler；
  支持 start/end event、userTask、scriptTask、排他/并行/包容网关、callActivity、multiInstance（会签/或签）
- **存量无缝迁移**：`format/windjson` 直接兼容 go-wind-oa / go-wind-im 的 node_config
  JSON（v2 图格式 + 旧版节点数组）
- **持久化驱动状态机**：每次流转落库，崩溃可恢复；定义不可变 + 版本锁定
- **零核心依赖**：`flow` 包不 import 任何第三方库；表达式/脚本/身份/存储全部 SPI
- **脚本引擎任意插拔**：`contrib/goscripts` 一套依赖接入 JS / Lua / Python / CEL / Expr 等 9 种引擎
- **企业语义内建**：多租户、会签（ALL）/ 或签（ANY）、委托代办、转办、撤回、
  超时打点、子流程 fail-closed 传播、全链路历史审计
- **fail-closed 安全观**：条件求值失败不误走分支；办理人解析失败不建任务；
  脚本沙箱白名单出口

## 快速开始

```bash
go get github.com/tx7do/go-wind-flow
```

```go
package main

import (
	"context"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	"github.com/tx7do/go-wind-flow/store/memory"
)

func main() {
	eng := flow.New(
		flow.WithStore(memory.New()),          // 生产换 ent 实现
		flow.WithImporters(bpmn.Importer{}),   // BPMN XML 导入
	)

	// 1. 部署（不可变，自动版本化）
	bpmnData, _ := os.ReadFile("examples/leave-approval.bpmn.xml")
	def, _ := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "leave", Data: bpmnData})

	// 2. 启动实例（同步推进到首个阻塞点）
	inst, tasks, _ := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "leave", StarterID: "u_emp",
		Variables: flow.Vars{"days": 5},
	})

	// 3. 办结任务（会签/或签/驳回语义由定义驱动）
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "boss", Outcome: flow.OutcomeApprove,
	})
	_ = inst
}
```

完整端到端示例见 `engine_test.go` 与 `examples/leave-approval.bpmn.xml`。

## 架构一览

```
┌──────────────────────────────────────────────────────────────┐
│  宿主应用（go-wind-oa / go-wind-im / go-wind-erp / CRM ...） │
└──────┬───────────────────────────────┬───────────────────────┘
       │ 引擎 API                       │ SPI 实现（按需装配）
┌──────▼───────────────────────────────▼───────────────────────┐
│                     flow（核心，零依赖）                      │
│  Engine / Deploy·Start·CompleteTask·Withdraw                 │
│  walker（Token 推进内核）   validate（部署校验）              │
│  ── SPI ──────────────────────────────────────────────────   │
│  Store  ExpressionEvaluator  ScriptRunner  IdentityProvider  │
│  Importer/Exporter  Listener  Clock/IDGen                    │
└──────▲──────────▲──────────▲──────────▲──────────────────────┘
       │          │          │          │
 store/memory  contrib/   contrib/   format/
 （嵌入式）     exprlang   goscripts  bpmn · windjson
               (expr-lang) (9 种脚本)  (BPMN XML · 存量 JSON)
```

## 与 go-scripts 的分工

| 场景 | 用什么 | 说明 |
|---|---|---|
| 网关条件、动态办理人 | `SimpleEvaluator`（内置）或 `contrib/exprlang` / go-scripts CEL/Expr | 确定性、无副作用、fail-closed |
| 脚本任务、监听器逻辑 | `contrib/goscripts`（JS/Lua/Python/Go/WASM…） | 图灵完备，沙箱 + 超时 |
| 高安全场景 | go-scripts 的 WASM (wazero) | 强隔离 |

## 文档

- [架构设计](docs/architecture.md) —— Token 状态机、事务边界、SPI 契约
- [BPMN 兼容策略](docs/bpmn-compat.md) —— 支持矩阵、扩展约定、与 bpmn-js/Camunda 互通
- [从 OA/IM 迁移指南](docs/migration.md) —— 双引擎对比、绞杀者路径、存量定义兼容
- [路线图](docs/roadmap.md) —— M0 → M4 里程碑

## License

MIT
