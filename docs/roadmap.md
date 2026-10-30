# 路线图（Roadmap）

> **定位**：M0 → M4 里程碑的唯一口径——已完成能力以本文勾选项为准；
> 新增能力先进对应里程碑、再动工。宿主集成关心的近期事项见 M1。

## M0 —— 核心骨架 ✅（当前提交）

- [x] Token 状态机内核：start/end、userTask、scriptTask、排他/并行/包容网关、callActivity
- [x] 会签（ALL）/ 或签（ANY）、驳回 TERMINATE/STAY、转办、撤回、强制终止
- [x] SPI 全集：Store / ExpressionEvaluator / ScriptRunner / IdentityProvider / Listener / Clock / IDGen
- [x] `store/memory` 内存实现（嵌入式可用）
- [x] `format/bpmn` BPMN 2.0 XML 导入导出（M0 子集）+ 回环测试
- [x] `format/windjson` OA/IM node_config 兼容（v2 图 + 旧版数组）
- [x] `contrib/exprlang`（expr-lang，与 OA/IM 行为一致）
- [x] `contrib/goscripts`（JS/Lua/CEL/Expr 等接入，真引擎冒烟测试通过）
- [x] 内置零依赖表达式求值器 + 部署校验 + 全链路历史事件

## M1 —— 生产化（目标：接入 go-wind-oa/im）

- [x] 生产 Store：`store/ent`（五表 schema + WithTx 事务，生成代码入库）
      与 `store/gorm`（多方言，MySQL/PostgreSQL/SQLite）——均已内置提供，
      独立子模块，主模块 go.sum 保持零负担
- [x] Protobuf 契约：`api` 独立子模块——FlowDefinition/FlowInstance/FlowTask/
      FlowHistory 四服务（**纯 gRPC，无 HTTP 注解**；HTTP 由宿主 BFF 层定义），
      buf 生成码入库，proto 发布 BSR（buf.build/go-wind/go-wind-flow），
      宿主 embed 或独立流程中心共用同一契约
- [x] 服务封装：`server/` 独立子模块——实现 api 四契约（纯 gRPC），proto ↔
      领域结构映射、错误码映射（NotFound/PermissionDenied/FailedPrecondition/…）、
      写路径 WithTx 事务接线；宿主任意 grpc.ServiceRegistrar 注册
      （kratos grpc server / 裸 grpc.Server）；HTTP 由宿主 BFF 层定义
- [x] 加签（AddTaskApprovers，会签式 + 委托叠加 + 去重）与驳回指定节点
      （ReturnTo，userTask 祖先、并行在途 fail-closed）；契约 v0.1.1 增补
      AddSigners / ReturnTo RPC（已推 BSR），server 层 RPC 级测试覆盖
- [x] 变量作用域：令牌级局部变量（Token.Locals，fork 快照克隆实现分支隔离）——
      读侧叠加（局部 > 实例），写侧脚本 `flow:scope="local"` / CompleteTask
      local_variables；不回写业务数据、随令牌消亡；契约 v0.1.3 增补
      local_variables（已推 BSR）
- [x] 定时器节点（`intermediateTimerEvent`，ISO8601 时长；令牌阻塞 + DueAt 打标，
      `ResumeDueTimers` 续跑）+ 调度参考实现 `examples/scheduler`（单 cron 循环
      同时驱动定时器续跑与任务到期扫描，经服务层走生产路径）；契约 v0.1.2
      增补 ResumeDueTimers RPC 与 TIMER_FIRED 事件（已推 BSR）
- [x] 设计器契约定型：`format/designer`（designer-json-v1，领域字段 + UI
      坐标/拐点，可直连 Deploy）+ 交互协议文档 [designer_contract.md](./designer_contract.md)
      （草稿/发布分离、错误回传、坐标保鲜）；go-wind-ui 按此开发
- [x] 性能基线（内存档）：`engine_bench_test.go`——线性流完整审批 ~104µs
      （≈9.6k ops/s 单核）、网关选路 ~29µs、十人会签全周期 ~6.6ms；
      Store 批量化与真实 DB 压测归入 M2

## M2 —— BPMN 子集扩充（目标：吸引 BPMN 用户）

- [x] boundaryEvent 边界定时事件（挂接 userTask）：中断型到期取消原任务并接管
      流程、非中断型派生升级分支（一次性触发）；主任务办结/实例终结时清理未触发
      边界令牌；BPMN 导入导出 attachedToRef/cancelActivity；可达性校验将边界
      事件作为扩散种子
- [x] 中间捕获事件：timer（M1）与 message（M2，SendMessage 解锁 + message.received 事件）已落地；抛出事件/signal 跨实例广播归 M3
- [x] 顺序多实例（isSequential=true）：解析序串行逐个审批（隐含 ALL），序列
      快照存令牌保留键 flow:miSeq/miDone（不进表达式环境），中途驳回走 RejectMode
- [x] documentation 透传（双向，XML 转义）+ BPMN DI 坐标透传（导出回填
      bpmndi 图形与原命名空间声明，bpmn-js/Camunda 视觉无损往返）
- [x] Camunda Modeler 往返兼容性测试集（带前缀根元素/incoming-outgoing/
      camunda: 属性/documentation/DI 全量回环）；任务超时自动动作
      （auto_approve / auto_reject，缺省仅广播）一并落地
- [x] bpmn-js properties-panel 扩展 + Vue3/React 双前端适配器（frontend/ npm 工作区，flow:assignees 可视化配置 + designer-json-v1 转换）
- [x] 候选组任务（candidateGroups）：GROUP 规则 → 待认领任务，Claim/Unclaim/
      组待办查询（IdentityProvider.ResolveGroupsForUser 鉴权）；契约 v0.1.5
- [x] 错误事件：errorEndEvent + callActivity 错误边界（errorRef 通配），
      未捕获 fail-closed 父实例 REJECTED
- [x] 集合多实例（collection 表达式 + elementVar，串行项变量入令牌局部）
- [x] 嵌入子流程扁平化导入（SES 约束，前缀并入，边界/MI 子流程显式拒绝）+
      定时器启动事件解析（调度宿主驱动）

## M3 —— 生态与商业化

- [ ] `contrib/`：ent store 独立模块、OPA 授权过滤（对接 kratos-authz）、
      go-scripts 沙箱策略模板（域名白名单/禁网模式）
- [ ] 行业模板库：请假/报销/采购/合同 会审模板（BPMN + 表单 + 规则）
- [ ] open-core 划线：企业版 = 多级审批代理规则、审计增强、SSO、SLA

## M4 —— 独立部署形态

- [ ] 独立流程中心服务（单二进制，兼容 XXL-Job 式接入习惯的执行器 SDK）
- [ ] 跨实例分布式令牌推进（存储行锁 → 分区锁）
- [ ] 管理控制台：定义版本管理、在途实例干预（跳转/改派/终止）、SLA 看板
