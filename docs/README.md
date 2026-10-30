# 文档索引（docs/）

本目录是 GoWind Flow 的全部项目文档入口。当前文档分**一层半**：参考层已就绪，
教程层随 M1（宿主接入实战）补齐。

| 层 | 位置 | 定位 | 读者 |
|---|---|---|---|
| **参考层** | 本目录各专题文档 | 每篇一个主题的**唯一权威**说明：完整机制、约定、迁移边界 | 动手集成或改内核前必读（集成者 / 维护者） |
| **教程层**（规划） | `tutorial/`（M1 补齐） | 循序渐进：从空仓库跑到第一个审批流上线 | 从零接手的采用者 |

**规则：两层冲突时，以参考层与代码为准。** 每个 SPI、每种 BPMN 扩展的行为
以 `flow/` 代码与对应参考文档为准，README 只负责路径与卖点。

---

## 参考层（按主题查阅）

| 文档 | 主题 | 什么时候读 |
|---|---|---|
| [architecture_design.md](./architecture_design.md) | 引擎架构：Token 状态机、事务边界、定义与版本、SPI 契约、fail-closed 语义表、目录结构 | 改 `flow/` 内核、为新宿主装配 SPI、排查"为什么没推进"之前 |
| [bpmn_compat.md](./bpmn_compat.md) | BPMN 兼容：元素支持矩阵、`<flow:assignees>` 扩展约定、UEL 条件映射、bpmn-js / Camunda 互通、格式插件机制 | 接存量 BPMN、接流程设计器、新增 Importer/Exporter 之前 |
| [designer_contract.md](./designer_contract.md) | 设计器契约：designer-json-v1 文档结构、字段语义、设计器↔服务端交互协议、演进规则 | go-wind-ui 设计器开发、BFF 保存/发布接口、契约升版之前 |
| [oa_im_migration.md](./oa_im_migration.md) | 迁移：go-wind-oa / go-wind-im 双引擎对比、SPI↔旧仓库适配层清单、绞杀者四阶段、行为差异清单 | 启动宿主迁移工单之前 |
| [roadmap.md](./roadmap.md) | 路线图：M0 → M4 里程碑口径 | 规划下一个能力、评估当前边界时 |

## 资产

| 位置 | 内容 |
|---|---|
| [brand/](./brand/) | 品牌图（flow-tile.svg，三语 README 共用） |
| [../examples/](../examples/) | 可部署样例：`leave-approval.bpmn.xml`（请假审批，含条件网关与或签） |

## 按角色的推荐路线

- **引擎采用者**（嵌入自己的 Go 应用）：根 [README](../README.md) 快速开始 → [architecture_design.md](./architecture_design.md)（重点 SPI 契约）→ 直接看 [engine_test.go](../engine_test.go) 照抄端到端用法。
- **宿主集成者**（接 go-wind-oa / im / erp）：[architecture_design.md](./architecture_design.md) → [oa_im_migration.md](./oa_im_migration.md)（适配层清单按表实现）→ [bpmn_compat.md](./bpmn_compat.md)（设计器契约）。
- **从 OA·IM 迁移的执行者**：[oa_im_migration.md](./oa_im_migration.md) 从头到尾，重点 §2 存量兼容与 §4 行为差异清单。
- **贡献者**：根 [AGENTS.md](../AGENTS.md)（架构红线）→ [architecture_design.md](./architecture_design.md) → [roadmap.md](./roadmap.md) 认领里程碑 → [CONTRIBUTING.md](../CONTRIBUTING.md)。
- **只想知道"我的 BPMN 能不能跑"**：直接 [bpmn_compat.md 支持矩阵](./bpmn_compat.md)。
