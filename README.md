<div align="center">

<img src="docs/brand/flow-tile.svg" width="120" alt="GoWind Flow｜川流" />

# GoWind Flow｜川流

**go-wind 家族的轻量流程引擎：持久化 Token 状态机 · BPMN 2.0 兼容 · 零核心依赖**

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)
[![BPMN](https://img.shields.io/badge/BPMN-2.0%20Compatible-4B8BBE)](./docs/bpmn_compat.md)
[![Scripts](https://img.shields.io/badge/go--scripts-9%20Engines-2E7D32)](https://github.com/tx7do/go-scripts)
[![Tests](https://img.shields.io/badge/Tests-Passing-brightgreen)](./engine_test.go)

[English](./README.en-US.md) | **中文** | [日本語](./README.ja-JP.md)

</div>

---

`go-wind-flow` 是 [go-wind](https://github.com/tx7do/go-wind) 生态的工作流底座，从
[go-wind-oa](https://github.com/tx7do/go-wind-oa) / go-wind-im 的审批流引擎提取、泛化而来，
并与 [go-scripts](https://github.com/tx7do/go-scripts) 深度配合——**引擎管流转，脚本管"流转之外的所有逻辑"**。

对标 Java 世界 Flowable / Camunda / Activiti 的常用子集，为 OA、ERP、CRM、低代码平台的
Go 私有部署市场提供审批流 / 业务流底座。

## 项目亮点

- **BPMN 2.0 兼容，无缝吸引 BPMN 用户**：`format/bpmn` 双向导入导出 BPMN XML，存量 BPMN 文件直接部署，
  导出结果可被 bpmn-js / Camunda Modeler 打开编辑；节点类型、网关语义、条件表达式风格与 BPMN 互认
  （支持矩阵见 [docs/bpmn_compat.md](./docs/bpmn_compat.md)）
- **存量流程零改写迁移**：`format/windjson` 直接兼容 go-wind-oa / go-wind-im 的 node_config JSON
  （v2 图格式 + 旧版节点数组），引擎按内容自动嗅探格式（迁移指南见 [docs/oa_im_migration.md](./docs/oa_im_migration.md)）
- **持久化驱动的 Token 状态机**：每次流转落库、崩溃可恢复，不做内存态执行；定义不可变 + 版本锁定，
  在途实例不受重部署影响
- **零核心依赖，积木式 SPI**：`flow` 包不 import 任何第三方库——存储、表达式、脚本、身份、格式全部
  SPI 接口，按需装配（契约见 [docs/architecture_design.md](./docs/architecture_design.md)）
- **脚本引擎任意插拔**：`contrib/goscripts` 一套依赖接入 go-scripts 的 JS / Lua / Python / CEL / Expr 等
  9 种引擎（纯 Go、零 CGO）；生产求值可换 `contrib/exprlang`（与 go-wind-oa/im 现网行为一致）
- **企业语义内建**：多租户、会签（ALL）/ 或签（ANY）、委托代办、转办、撤回、超时打点、
  子流程 fail-closed 传播、全链路历史审计
- **fail-closed 安全观**：条件求值失败不误走分支、办理人解析失败不建任务、脚本沙箱白名单出口，
  所有不确定态一律拒绝而非放行

## 从这里开始

- **想直接上手**：读 [快速开始](#快速开始)，跑通 [examples/leave-approval.bpmn.xml](./examples/leave-approval.bpmn.xml) 请假审批全链路。
- **想系统理解引擎**：读 [文档索引](./docs/README.md)——按你的角色（引擎采用者 / 宿主集成者 /
  从 OA·IM 迁移者 / 贡献者）标好了阅读路径，参考层每篇是一个主题的唯一权威说明。
- **想知道"能不能导入我的 BPMN"**：直接看 [BPMN 支持矩阵](./docs/bpmn_compat.md#2-支持矩阵m0)。

## 代码仓库

| 平台 | 地址 |
|------|------|
| GitHub | <https://github.com/tx7do/go-wind-flow> |
| Gitee | 同步中 |
| AtomGit | 同步中 |

## 快速开始

### 环境要求

| 工具 | 版本 |
|------|------|
| Go | 1.25+ |
| （可选）go-scripts | 本地同级目录或 GitHub 拉取，启用脚本任务 / CEL·Expr 求值时需要 |

### 安装

```bash
go get github.com/tx7do/go-wind-flow
```

### 三步跑通请假审批

```go
package main

import (
	"context"
	"os"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	"github.com/tx7do/go-wind-flow/store/memory"
)

func main() {
	ctx := context.Background()
	eng := flow.New(
		flow.WithStore(memory.New()),        // 生产换 ent 实现
		flow.WithImporters(bpmn.Importer{}), // BPMN XML 导入
	)

	// 1. 部署（不可变，自动版本化）
	bpmnData, _ := os.ReadFile("examples/leave-approval.bpmn.xml")
	_, _ = eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "leave", Data: bpmnData})

	// 2. 启动实例（同步推进到首个阻塞点）
	inst, tasks, _ := eng.Start(ctx, flow.StartRequest{
		TenantID:  "t1",
		Key:       "leave",
		StarterID: "u_emp",
		Variables: flow.Vars{"days": 5},
	})

	// 3. 办结任务（会签/或签/驳回语义由定义驱动）
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "boss", Outcome: flow.OutcomeApprove,
	})
	_ = inst
}
```

### 运行测试

```bash
go test ./...        # 核心引擎 + 格式适配器（含 BPMN 导入导出回环测试）
make tidy            # 三层 go.mod 依次 tidy（core / contrib/exprlang / contrib/goscripts）
```

## 架构一览

```
┌──────────────────────────────────────────────────────────────┐
│  宿主应用（go-wind-oa / go-wind-im / go-wind-erp / CRM ...） │
└──────┬───────────────────────────────┬───────────────────────┘
       │ 引擎 API                       │ SPI 实现（按需装配）
┌──────▼───────────────────────────────▼───────────────────────┐
│                     flow（核心，零依赖）                      │
│  Engine / Deploy·Start·CompleteTask·Withdraw                 │
│  walker（Token 推进内核）    validate（部署校验）             │
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
| 网关条件、动态办理人 | `SimpleEvaluator`（内置）或 `contrib/exprlang` / go-scripts CEL·Expr | 确定性、无副作用、fail-closed |
| 脚本任务、监听器逻辑 | `contrib/goscripts`（JS/Lua/Python/Go/WASM…） | 图灵完备，沙箱 + context 控时 |
| 高安全场景 | go-scripts 的 WASM（wazero） | 强隔离 |

## 在 go-wind 家族中的位置

| 项目 | 关系 |
|------|------|
| [go-wind](https://github.com/tx7do/go-wind) | 微服务内核：本引擎遵循其"积木盒"设计哲学（零隐藏魔法、SPI 组装） |
| [go-wind-admin](https://github.com/tx7do/go-wind-admin) | 企业级中后台脚手架：流程引擎可为其提供审批流底座 |
| [go-wind-oa](https://github.com/tx7do/go-wind-oa) / go-wind-im | 审批流引擎的原生宿主，迁移路径见 [docs/oa_im_migration.md](./docs/oa_im_migration.md) |
| [go-scripts](https://github.com/tx7do/go-scripts) | 嵌入式脚本框架：本引擎的脚本任务与表达式求值实现方 |

## 文档

全部文档入口：[docs/README.md](./docs/README.md)

| 文档 | 主题 |
|------|------|
| [architecture_design.md](./docs/architecture_design.md) | 引擎架构：Token 状态机、事务边界、SPI 契约、fail-closed 语义表 |
| [bpmn_compat.md](./docs/bpmn_compat.md) | BPMN 兼容：支持矩阵、办理人扩展约定、与 bpmn-js / Camunda 互通 |
| [oa_im_migration.md](./docs/oa_im_migration.md) | 迁移指南：双引擎对比、绞杀者路径、存量定义兼容、行为差异清单 |
| [roadmap.md](./docs/roadmap.md) | 路线图：M0 核心骨架（当前）→ M1 生产化 → M2 BPMN 扩充 → M3 生态 → M4 独立部署 |

## 路线图

当前处于 **M0（核心骨架）**——引擎内核、格式适配、SPI 全集已就绪并通过端到端测试；
下一站 M1 生产化（ent Store、kratos 服务封装、go-wind-ui 流程设计器契约），
完整里程碑见 [docs/roadmap.md](./docs/roadmap.md)。

## License

[MIT](./LICENSE)
