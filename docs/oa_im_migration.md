# 从 go-wind-oa / go-wind-im 迁移指南

## 1. 双引擎对比结论（2026-10）

两处现有引擎同源，**go-wind-im 版本更完整**（2026-09-04，47KB vs 40KB），
go-wind-flow 以 IM 版为提取蓝本：

| 能力 | go-wind-oa<br>(core/service) | go-wind-im<br>(oa/service) | go-wind-flow |
|---|---|---|---|
| 图模型 | START/END/TASK/排他/并行分叉汇合/子流程 | 同左 | 对齐 BPMN 命名 + 包容网关（新增） |
| 办理人 | USER/LEADER/POSITION/ROLE + 委托 | 同左 | 同左 + STARTER/EXPR |
| 会签/或签 | ALL/ANY | 同左 | 同左（含 RejectMode 显式化） |
| 加签/转办/撤回 | ✅ | ✅ | 转办/撤回 ✅（加签 M1） |
| 子流程父链接 | ✅ | ✅ | ✅（callActivity） |
| 超时调度 | ✅ scheduler | ✅ | DueAt 打标 + CheckDueTasks（调度归宿主） |
| 条件求值 | expr-lang 沙箱 | expr-lang 沙箱 | SimpleEvaluator 内置 / contrib-exprlang 同行为 |
| 持久化 | ent | ent | Store SPI（memory 内置，ent 在宿主） |
| BPMN 兼容 | ❌ | ❌ | ✅ 导入导出 |

提取时的关键泛化：

- **ID 从 uint32 → string**：核心与存储解耦；宿主在 IdentityProvider/Store 实现里
  做 `uint32 ↔ string` 映射。
- **节点类型改为 BPMN 命名**：`EXCLUSIVE_GATEWAY → exclusiveGateway` 等，
  windjson 导入器负责存量映射。
- **审批人 JSON 从 `{type,id,code}` → `{kind,value}`**：windjson 导入器自动转换，
  存量 node_config 无需改写。

## 2. 存量定义兼容

`format/windjson` 保证零迁移成本：

- **v2 图格式**（`{"version":2,"nodes":[...],"edges":[...]}`）——完整映射，
  含 `PARALLEL_GATEWAY_FORK/JOIN → parallelGateway`、
  `condition:"default" → IsDefault`、`subprocessDefinition:"code:3" → key+version`；
- **旧版节点数组**（`[{"approver_type":"USER",...}]`）——自动补 START/END
  构造等价线性图（与 IM `buildGraphFromLegacy` 行为一致）。

引擎格式嗅探：数据以 `<` 开头按 BPMN XML 处理，否则按 wind-json-v2——
存量宿主把 `node_config` 原文丢给 `Deploy` 即可。

## 3. 绞杀者迁移路径（宿主侧操作）

```
阶段 0（并行运行）    新引擎以适配器实现旧 WorkflowService 的存储接口，
                      新部署的定义双写 node_config JSON（保证可回滚）
阶段 1（增量切换）    新建流程走 go-wind-flow；在途实例留在旧引擎跑完
阶段 2（定义迁移）    批量把存量定义经 windjson.Importer 部署到新引擎（自动升版本）
阶段 3（收尾）        旧引擎功能冻结 → 在途清零 → 删除旧代码与表
```

适配层清单（宿主实现）：

| go-wind-flow SPI | OA/IM 现有对应物 |
|---|---|
| `Store.Definitions()` | `workflow_definition_repo.go` |
| `Store.Instances()` | `workflow_instance_repo.go` + join/parent_link |
| `Store.Tokens()` | 无（旧引擎无令牌概念，需新表或由 instance 派生） |
| `Store.Tasks()` | `workflow_task_repo.go` |
| `Store.History()` | `workflow_log_repo.go` |
| `IdentityProvider` | `workflow_resolver_repo.go`（LEADER/POSITION）+ role resolver + `delegationRepo` |
| `ExpressionEvaluator` | `workflow_condition_eval.go` → 直接换 `contrib/exprlang` |
| `Listener` | `workflow_hook.go` 的 WorkflowEventRegistry → 终结回调改为监听器 |
| 超时调度 | `workflow_timeout_scheduler.go` → cron 调 `CheckDueTasks` |

## 4. 行为差异清单（切换前过一遍）

1. **驳回**：默认语义一致（TERMINATE）；`RejectMode: STAY` 为新增可选。
2. **网关校验**：go-wind-flow 部署时强校验（可达性、默认流唯一等），
   比旧引擎更严格——存量定义先在测试环境 `Deploy` 验证。
3. **并行网关**：FORK/JOIN 合并为单一 `parallelGateway` 类型，校验规则等价。
4. **包容网关为新增能力**，旧定义不受影响。
5. **令牌表**：新引擎引入 Token 实体（旧引擎无），Store 实现需建新表
   （ent schema 建议：id/instance_id/node_id/status/parent_id/child_inst_id/created_at）。
6. **uint32 ID**：`IdentityProvider.ResolveApprovers` 返回 string；
   宿主转换 `strconv.FormatUint(uint64(uid), 10)`。
