# 架构设计

## 1. 定位与设计原则

go-wind-flow 是**库优先**的流程引擎：核心 `flow` 包零第三方依赖，可嵌入任意
Go 进程（OA/ERP/CRM 服务、独立调度中心），也可被包装成 gRPC 服务对外提供。
四条原则贯穿实现：

1. **积木盒而非全家桶**（与 go-wind 家族一致）——所有外部能力都是 SPI，具体实现按需装配；
2. **对齐 BPMN**——节点类型、网关语义、条件风格与 BPMN 2.0 互认，降低用户迁移与心智成本；
3. **持久化驱动**——不做内存态执行，每次流转落库，崩溃可恢复；
4. **fail-closed**——所有不确定态（条件求值失败、办理人解析为空、子流程异常）一律拒绝/驳回，绝不误放行。

## 2. 执行模型：Token 状态机

```
Instance (1) ──< Token (n)          Token：实例在图中的游标
    │                                   状态：RUNNING → WAITING → ENDED
    ├──< Task (n)                   Task：用户任务（一个 Token 可产多个，会签/或签）
    └──< HistoryEvent (append-only) 历史：全链路审计
```

- **推进内核 `runToken`**：从当前节点出发同步循环推进，直到阻塞
  （用户任务/子流程/汇合等待）或实例终结。每次迁移即时 `store.Update` 落库。
- **并行网关**：BPMN 惯例——同一元素按出入度区分分叉/汇合。分叉生成分裂令牌
  （`ParentID` 指向源令牌）；汇合按"到达数 ≥ 入度"吸收等待令牌。
- **包容网关**：分叉时按条件选分支并把期望到达数记入实例变量
  `flow:joinExpected:<nodeID>`；汇合按该数吸收。
  *限制：同一汇合节点同时只支持一个活跃分叉（v0 文档化约束）。*
- **子流程（callActivity）**：父令牌阻塞并记录 `ChildInstID`；子实例终结时
  fail-closed 传播——`APPROVED` 恢复父令牌（沿出边离开 callActivity 节点，
  防止重入），非 `APPROVED` 一律令父实例 `REJECTED`。
- **会签/或签**：多实例语义收敛在 Task 层——ALL 等全部通过、ANY 首个通过即
  取消其余；任一驳回按 `RejectMode` 处理（默认 TERMINATE，与 OA/IM 现行为一致）。

### 步长保护

`runToken` 单次推进有 `maxSteps`（默认 10000）上限，防御异常定义造成的死循环。

## 3. 事务边界

引擎自身无锁、无事务。约定：

- **进程内**：宿主对同一实例的引擎入口（Start / CompleteTask / Withdraw / Abort）
  按 instanceID 串行化。子流程经父实例调用链天然串行。
- **生产 Store（ent）**：建议在每个引擎入口包一层事务（单实例单事务），
  行级锁兜底并发。内存实现用 RWMutex。

引擎入口少而清晰，这是刻意设计：事务边界只出现在入口，内部推进不跨事务。

## 4. 定义与版本

- **部署即版本化**：`Deploy` 生成 `version = max + 1`，定义不可变；
- **实例锁定版本**：实例创建时记录 `DefinitionVersion`，改定义只影响新实例；
- **源文本保留**：`Definition.Source` 存原始 XML/JSON，审计与导出随时回放。

## 5. SPI 契约

| SPI | 职责 | 内置/参考实现 |
|---|---|---|
| `Store` | 定义/实例/令牌/任务/历史 五类存储 | `store/memory`；生产建议 ent |
| `ExpressionEvaluator` | 布尔表达式（网关条件、动态办理人） | `SimpleEvaluator`（内置零依赖）；`contrib/exprlang`；go-scripts CEL/Expr |
| `ScriptRunner` | 脚本任务执行 | `contrib/goscripts`（JS/Lua/Python/Go/WASM…） |
| `IdentityProvider` | 办理人规则解析 + 委托代办 | 核心内置静态解析（USER/STARTER）；LEADER/POSITION/ROLE/EXPR 由宿主实现 |
| `Importer/Exporter` | 定义格式双向转换 | `format/bpmn`、`format/windjson` |
| `Listener` | 生命周期事件（监听器） | 注册即用；panic 被 recover 隔离 |
| `Clock` / `IDGen` / `Logger` | 时间/ID/日志 | 默认实现可替换，测试注入假时钟 |

### fail-closed 语义一览

| 场景 | 行为 |
|---|---|
| 排他网关条件求值失败 | 返回错误，不推进（绝不误走分支） |
| 无分支命中且无默认流 | 错误 |
| 办理人解析失败/为空 | 不建任务，令牌留在原地，由宿主决定重试/终止 |
| 未装配 IdentityProvider 却遇到动态规则 | `ErrNotConfigured` |
| 脚本任务执行失败 | 错误 + 历史记录，令牌不推进 |
| 子流程 REJECTED/WITHDRAWN/ABORTED | 父实例 REJECTED（与 OA/IM 现行为一致） |

## 6. 与 go-scripts 的结合点

- 引擎核心只认 `ExpressionEvaluator` / `ScriptRunner` 两个小接口，**不 import go-scripts**；
- 脚本环境仅注入实例变量 + 宿主注册的白名单函数；控时走 context deadline；
- 脚本任务返回 `map[string]any` 时键值自动合并进实例变量（触发 `variable.changed`）；
- 超时任务：定义带 `timer` 时任务打 `DueAt` 标，宿主 cron 驱动
  `CheckDueTasks` 广播 `task.due` 事件，催办/升级/自动处理交给监听器。

## 7. 目录结构

```
flow/            核心引擎（零依赖）
  flow.go        错误/日志/时钟/IDGen
  model.go       Definition/Node/Edge + 规格
  runtime.go     Instance/Token/Task/Event
  spi.go         全部 SPI 接口
  simple_expr.go 内置表达式求值器
  validate.go    部署校验
  engine.go      引擎入口 API + 实例终结
  walker.go      Token 推进内核（网关/任务/脚本/子流程）
  importer.go    Importer/Exporter + 注册表
store/memory/    内存实现
format/bpmn/     BPMN 2.0 XML 双向转换
format/windjson/ OA/IM node_config 兼容
contrib/exprlang/  expr-lang 求值器（独立子模块）
contrib/goscripts/ go-scripts 适配（独立子模块）
examples/        示例 BPMN
```
