# AGENTS.md

go-wind-flow：go-wind 家族的流程引擎。本文件给 AI 编码代理与贡献者的约定。

## 语言与注释

- 代码注释使用中文，与 go-wind 家族仓库保持一致；注释解释"为什么"与约束，
  不复述代码。
- 错误信息使用英文（`fmt.Errorf`），哨兵错误集中在 `flow/flow.go`。

## 架构红线（改动前必读）

1. **`flow/` 核心包零第三方依赖**。任何新依赖（含标准库以外）必须放进
   `store/`、`format/`、`contrib/` 等外围包，通过 `flow/spi.go` 的接口接入。
2. **引擎核心不 import go-scripts / go-wind 运行时**。表达式、脚本、身份、
   存储全部走 SPI；contrib 为独立 go.mod 子模块。
3. **fail-closed**：条件求值失败不推进；办理人解析为空不建任务；
   子流程非 APPROVED 时父实例一律 REJECTED。不要"宽容降级"。
4. **定义不可变**：Deploy 生成新版本；在途实例锁定版本号。
5. **持久化驱动**：每次状态迁移即时落库；禁止内存态批量化后延迟写。
6. **并发约定**：引擎无锁；同一实例的入口调用由宿主串行化。
   内部代码不得新增实例级互斥锁（子流程调用链会死锁）。

## 测试

- 核心行为变更必须带测试；端到端场景写在根目录 `engine_test.go`
  （BPMN 部署 → 启动 → 网关 → 会签 → 完成 全链路）。
- 格式适配器用 `examples/` 下的样例文件做回环测试（Parse → Write → Parse）。

## 文档

- 三语 README（`README.md` 中文主文档 / `README.en-US.md` / `README.ja-JP.md`）
  结构保持一致，改主文档后同步另两份。
- `docs/README.md` 是文档索引；参考层每篇是一个主题的唯一权威说明，
  对外行为变更必须同步对应参考文档与 `CHANGELOG.md`（Unreleased）。
- 专题文档命名用 snake_case（家族约定），文首用 `> **定位**` 引导块。

## 常用命令

```bash
go test ./...        # 核心测试
go vet ./...         # 静态检查
make tidy            # 三层 go.mod 依次 tidy
```

## 提交规范

Conventional Commits：`feat(flow): ...`、`fix(bpmn): ...`、`docs: ...`。
