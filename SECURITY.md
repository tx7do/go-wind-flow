# 安全策略

## 支持版本

| 版本 | 支持 |
|------|------|
| 0.1.x | ✅ |

## 安全设计基线（了解引擎的信任边界）

- **fail-closed**：条件求值失败不推进、办理人解析为空不建任务、子流程非 APPROVED
  时父实例一律驳回——不确定态不放行。
- **表达式沙箱**：内置 `SimpleEvaluator` 无 I/O、无函数调用；`contrib/exprlang`
  默认 `DisableAllBuiltins` 并锁定 Env。不要在网关条件里放开任意函数。
- **脚本沙箱**：`contrib/goscripts` 的脚本环境只注入实例变量与宿主显式注册的
  白名单函数；控时走 context deadline。生产高并发请包引擎池，并按
  go-scripts 的 hostmodule 启用 HTTP 出站白名单。
- **定义不可变**：在途实例锁定部署版本，防止定义被篡改后影响存量实例。

## 报告漏洞

**请勿通过公开 issue 报告安全漏洞。** 请邮件联系 <yanglinbo@gmail.com>，
主题注明 `[go-wind-flow security]`，附复现步骤与影响评估。

- 72 小时内确认收到；尽力 30 天内给出修复或缓解方案；
- 修复发布后在 CHANGELOG 与 release note 中致谢报告者（可要求匿名）。

## 范围说明

本仓是**库**：运行期安全（网络边界、存储加密、租户鉴权）由宿主应用与
go-wind 家族的网关/权限体系承担，不在本仓威胁模型内。
