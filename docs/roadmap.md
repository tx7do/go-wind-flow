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
      FlowHistory 四服务（gRPC + kratos HTTP 注解），buf 生成码入库，宿主 embed
      或独立流程中心共用同一契约
- [ ] kratos 服务封装（实现 api 契约，proto ↔ 领域结构映射 + WithTx 接线）
- [ ] 加签（动态增加办理人）、驳回指定节点（ReturnTo）
- [ ] 变量作用域（实例级 → 节点级局部变量）
- [ ] 定时器节点（`intermediateTimerEvent`）+ 宿主调度器参考实现
- [ ] 设计器协议定型：与 go-wind-ui 流程设计器（BPMN-JS 封装）的 JSON 契约
- [ ] 性能基线：单实例万级流转压测 + Store 批量化

## M2 —— BPMN 子集扩充（目标：吸引 BPMN 用户）

- [ ] boundaryEvent（边界定时/消息）+ 事件驱动分支
- [ ] 中间捕获/抛出事件（timer/message/signal）
- [ ] 顺序多实例（isSequential=true）
- [ ] documentation 透传、BPMN DI 坐标透传（导出保留图形）
- [ ] bpmn-js properties-panel 扩展（flow:assignees 可视化配置）
- [ ] Camunda Modeler 往返兼容性测试集

## M3 —— 生态与商业化

- [ ] `contrib/`：ent store 独立模块、OPA 授权过滤（对接 kratos-authz）、
      go-scripts 沙箱策略模板（域名白名单/禁网模式）
- [ ] 行业模板库：请假/报销/采购/合同 会审模板（BPMN + 表单 + 规则）
- [ ] open-core 划线：企业版 = 多级审批代理规则、审计增强、SSO、SLA

## M4 —— 独立部署形态

- [ ] 独立流程中心服务（单二进制，兼容 XXL-Job 式接入习惯的执行器 SDK）
- [ ] 跨实例分布式令牌推进（存储行锁 → 分区锁）
- [ ] 管理控制台：定义版本管理、在途实例干预（跳转/改派/终止）、SLA 看板
