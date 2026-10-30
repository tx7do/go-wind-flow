# Changelog

本文件记录 go-wind-flow 的对外可见变更。格式遵循
[Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循语义化版本。

## [Unreleased]

### Added

- `api`：Protobuf 服务契约（独立子模块）——FlowDefinition / FlowInstance /
  FlowTask / FlowHistory 四服务，gRPC + kratos HTTP 注解，分页复用
  go-crud `pagination.PagingRequest`；buf 生成码（go / go-grpc / go-http）入库，
  `make proto` 重新生成
- `store/ent`：flow.Store 的 ent 生产实现（独立子模块）——definitions/instances/
  tokens/tasks/history 五表 schema、`WithTx` 事务模式（引擎入口包裹，嵌套安全）、
  ent 生成代码入库；codegen 注意：entgo v0.14 loader 与 Go 1.27 不兼容，
  需 `GOTOOLCHAIN=go1.25.0` 生成
- `store/gorm`：flow.Store 的 GORM 生产实现（独立子模块）——MySQL/PostgreSQL/
  SQLite 多方言通用，JSON 字段经 gorm serializer 落库，同样的 `WithTx` 事务模式
- `store/memory`：补 `WithTx` 直通占位，宿主切换存储时引擎入口包装代码不变

### 计划（M1 生产化剩余）

- kratos 服务封装（gRPC + REST，Protobuf 契约）
- 加签（动态增加办理人）、驳回指定节点（ReturnTo）
- 定时器节点与宿主调度器参考实现
- go-wind-ui 流程设计器 JSON 契约定型

## [0.1.0] - 2026-10-30

### Added（M0 核心骨架）

- `flow` 核心引擎（零第三方依赖）：Token 状态机内核，start/end event、
  userTask、scriptTask/serviceTask、排他/并行/包容网关、callActivity 子流程
- 审批语义：会签（ALL）/ 或签（ANY）、驳回 TERMINATE/STAY、转办、撤回、
  强制终止、委托代办叠加、超时 DueAt 打标与到期扫描
- 子流程 fail-closed 传播（子实例非 APPROVED → 父实例 REJECTED）
- SPI 全集：Store / ExpressionEvaluator / ScriptRunner / IdentityProvider /
  Importer·Exporter / Listener / Clock / IDGen，含内置静态身份解析与
  零依赖表达式求值器（SimpleEvaluator）
- `store/memory`：进程内存储实现（嵌入式可用）
- `format/bpmn`：BPMN 2.0 XML 双向转换（含 `<flow:assignees>` 扩展、
  assignee/candidateUsers 属性兜底、UEL 条件剥壳、会签/或签映射），
  bpmn-js / Camunda Modeler 可直接打开导出结果
- `format/windjson`：go-wind-oa / go-wind-im node_config 存量格式兼容
  （v2 图格式 + 旧版节点数组）
- `contrib/exprlang`：expr-lang 求值器（与 go-wind-oa/im 现网行为一致，独立子模块）
- `contrib/goscripts`：go-scripts 适配器（JS/Lua/CEL/Expr 等，独立子模块）
- 端到端测试：BPMN 部署 → 启动 → 网关 → 会签/或签 → 完成 → 子流程父恢复 全链路
- 文档：三语 README（zh/en/ja）、架构设计、BPMN 兼容、迁移指南、路线图

[Unreleased]: https://github.com/tx7do/go-wind-flow/compare/0.1.0...HEAD
[0.1.0]: https://github.com/tx7do/go-wind-flow/releases/tag/0.1.0
