# Changelog

本文件记录 go-wind-flow 的对外可见变更。格式遵循
[Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循语义化版本。

## [Unreleased]

### Added

- 引擎：**中间消息事件**（`intermediateCatchEvent` + `messageEventDefinition`）——
  令牌阻塞等外部投递，`SendMessage(实例, 消息名, payload)` 解锁并续走
  （payload 并入实例变量，广播 `message.received`）；v1 按实例+消息名关联、
  引擎不缓存消息（投递失败由调用方自决重试）。BPMN 导入导出
  `messageEventDefinition/messageRef`；契约 v0.1.4 增补 SendMessage RPC
  （已推 BSR）
- 性能基线补充：ent + sqlite 真实存储档——完整审批周期（WithTx 事务 +
  五表落库）~1.1ms/次（`store/ent` BenchmarkEntLinearFlow），与内存档
  ~104µs 对照
- 引擎：**顺序多实例**（BPMN `isSequential=true`）——办理人按解析顺序串行
  逐个审批（同一时刻仅一个待办），全部通过才推进，中途驳回按 RejectMode；
  序列快照存令牌保留键（`flow:miSeq/miDone`，不进表达式环境）
- BPMN：**documentation 双向透传**（XML 转义）+ **DI 图形回填**——导出时从
  源定义回收 `bpmndi:BPMNDiagram` 与 definitions 原命名空间声明，
  对 bpmn-js / Camunda Modeler 视觉无损往返
- 引擎：**边界定时事件**（boundaryEvent，挂接 userTask）——中断型（缺省，
  cancelActivity=true）到期取消原任务全部待办并沿边界流出边接管流程；
  非中断型（cancelActivity=false）只派生升级分支（v1 一次性触发），原任务照常；
  主任务办结推进/实例终结时自动清理未触发的边界令牌。复用 ResumeDueTimers
  调度（无需新 API）；BPMN 导入导出 attachedToRef / cancelActivity；
  可达性校验将边界事件作为扩散种子。BPMN 矩阵已更新
- 引擎：**变量作用域**——令牌级局部变量（Token.Locals）：并行分支 fork 快照
  克隆实现隔离；读侧环境 = 实例 + 局部叠加（局部覆盖），网关条件/动态办理人/
  脚本任务统一生效；写侧经脚本任务 `flow:scope="local"` 或 CompleteTask 的
  LocalVariables；局部不回写业务数据、不产生 variable.changed、随令牌消亡。
  BPMN 导入导出 `flow:scope` 扩展属性（兼容带/不带命名空间声明两种形态）。
  契约 v0.1.3：CompleteTask 增 local_variables（已推 BSR）
- 设计器契约：`format/designer`（designer-json-v1）——领域字段 + UI 元数据
  （节点坐标/边拐点），设计器文档可直连 Deploy；契约与交互协议文档
  `docs/designer_contract.md`（草稿/发布分离、错误回传、坐标保鲜、演进规则）
- 性能基线：`engine_bench_test.go`（内存存储）——线性流完整审批 ~104µs、
  网关选路 ~29µs、十人会签全周期 ~6.6ms；真实 DB 压测归 M2
- 引擎：**定时器节点** `intermediateTimerEvent`——令牌到达即阻塞并打 DueAt 标，
  `ResumeDueTimers`（宿主 cron 驱动）到期续跑并广播 `timer.fired` 事件；
  BPMN 导入导出 `timerEventDefinition/timeDuration`（ISO8601，仅天/时/分/秒，
  年月周日历单位显式拒绝）；Token 实体新增 due_at（三处 store 同步）
- 调度参考实现 `examples/scheduler`：单 cron 循环驱动两条时间线
  （定时器续跑 + 任务到期扫描），经 flowserver 服务层走生产路径
- `api` 契约 v0.1.2：`FlowInstanceService` 新增 `ResumeDueTimers` RPC 与
  `TIMER_FIRED` 事件枚举（已推 BSR）；`server` 实现对应 RPC
- 引擎：**加签** `AddTaskApprovers`（向待办节点动态追加办理人，会签式、委托叠加、
  自动去重）与 **退回指定节点** `ReturnTo`（退回 userTask 祖先重走，动态办理人
  重新解析；非祖先 / 非 userTask / 在途并行分支均 fail-closed 拒绝）
- 事件：`task.approver_added`、`instance.returned` 两个生命周期事件（历史 + 监听器）
- `api` 契约 v0.1.1：`FlowTaskService` 新增 `AddSigners` / `ReturnTo` RPC 与
  对应枚举值（已推 BSR）
- `server`：两个 RPC 的服务实现与事件枚举映射，RPC 级端到端测试

- `server`：api 契约的 gRPC 服务实现（独立子模块）——proto ↔ 领域结构映射、
  哨兵错误 → gRPC 状态码映射（NotFound / PermissionDenied / FailedPrecondition /
  InvalidArgument / Internal）、写路径 WithTx 事务接线；`RegisterAll` 一次注册
  四服务到任意 grpc.ServiceRegistrar；端到端测试覆盖生命周期、错误码与查询
- `api`：Protobuf 服务契约（独立子模块）——FlowDefinition / FlowInstance /
  FlowTask / FlowHistory 四服务，**纯 gRPC 契约、无 HTTP 注解**（HTTP 路由由
  各宿主 BFF 层定义，家族惯例），分页复用 go-crud `pagination.PagingRequest`；
  buf 生成码（go / go-grpc）入库，`make proto` 重新生成；proto 同步发布 BSR
  **buf.build/go-wind/go-wind-flow**（label 与 git tag 对齐，v0.1.0）
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
