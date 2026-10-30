# 贡献指南

感谢关注 GoWind Flow！提交 PR 前请花两分钟读完本页。

## 开发环境

| 工具 | 版本 |
|------|------|
| Go | 1.25+ |
| （可选）golangci-lint | 最新版 |

```bash
go test ./...        # 核心测试（必过）
go vet ./...         # 静态检查（必过）
make tidy            # 三层 go.mod 依次 tidy（core / contrib/exprlang / contrib/goscripts）
```

注意 `contrib/` 下是**独立 go.mod 子模块**：改到其中代码时需进入对应目录单独
`go test ./...`；本地依赖 go-wind-flow 核心经 `replace => ../../`，依赖
go-scripts 经 `replace => ../../../go-scripts`（同级仓库）。

## 提交前自检

1. **架构红线**（完整清单见根 [AGENTS.md](./AGENTS.md)，违反会被拒）：
   - `flow/` 核心包零第三方依赖——新能力一律走 SPI 放外围包；
   - fail-closed：不要"宽容降级"，不确定态一律拒绝；
   - 定义不可变：Deploy 生成新版本，不改在途实例的定义；
   - 持久化驱动：每次状态迁移即时落库；
   - 引擎内部禁止新增实例级互斥锁（子流程调用链会死锁）。
2. **测试**：核心行为变更必须带测试；端到端场景写在根 `engine_test.go`；
   格式适配器用 `examples/` 样例做回环测试。
3. **文档**：改了对外行为，同步更新对应参考文档与三语 README
   （`README.md` 为主文档，`README.en-US.md` / `README.ja-JP.md` 跟随同步），
   并在 [CHANGELOG.md](./CHANGELOG.md) 的 Unreleased 下加一行。

## 提交规范

Conventional Commits：`feat(flow): ...`、`fix(bpmn): ...`、`docs: ...`、
`test(engine): ...`。PR 标题同理。

## 报告问题

提 issue 时请附：Go 版本、最小复现（BPMN/JSON 定义 + 调用序列）、
期望与实际行为。流程实例类问题附 `ListHistory` 输出会大幅加快定位。
