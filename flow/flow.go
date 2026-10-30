// Package flow 是 go-wind-flow 的核心引擎：一个持久化驱动的、
// 基于 Token 状态机的轻量流程引擎。
//
// 设计原则（与 go-wind 家族一致——积木盒而非全家桶）：
//
//   - 核心零第三方依赖：表达式、脚本、身份、存储全部为 SPI 接口，
//     具体实现位于 store/、format/、contrib/ 等独立包中按需组装；
//   - 对齐 BPMN：节点类型、网关语义、条件表达式风格与 BPMN 2.0 互认，
//     通过 format/bpmn 可无缝导入导出；
//   - 持久化驱动：每次流转都落库，崩溃可恢复，不做内存态执行；
//   - 定义不可变：部署即版本化，在途实例锁定其部署版本。
//
// 最小用法：
//
//	eng := flow.New(
//	    flow.WithStore(memstore.New()),
//	    flow.WithImporters(bpmn.Importer{}),
//	)
//	def, _ := eng.Deploy(ctx, flow.DeployRequest{Key: "leave", Data: bpmnBytes})
//	inst, _ := eng.Start(ctx, flow.StartRequest{Key: "leave", StarterID: "u1"})
package flow

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

// Logger 引擎最小日志门面。宿主可用 slog / zap / kratos log 适配。
type Logger interface {
	Printf(format string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Printf(string, ...any) {}

// Clock 时钟接口，测试可注入假时钟。
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// IDGen 标识生成器。默认 crypto/rand 随机 hex。
type IDGen func() string

func defaultIDGen() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极端情况下退化为纳秒时间戳，保证不重复。
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}

// 核心哨兵错误。宿主可用 errors.Is 判断；包装后仍可识别。
var (
	// ErrNotFound 资源不存在（定义/实例/任务/令牌）。
	ErrNotFound = errors.New("flow: not found")
	// ErrInvalidDefinition 定义不合法（校验失败）。
	ErrInvalidDefinition = errors.New("flow: invalid definition")
	// ErrInvalidState 非法状态迁移（如对已办结任务 Complete）。
	ErrInvalidState = errors.New("flow: invalid state")
	// ErrForbidden 操作者无权执行该操作（如非办理人办结任务）。
	ErrForbidden = errors.New("flow: forbidden")
	// ErrNotConfigured 缺少必要的 SPI 实现（如未装配 IdentityProvider 却遇到 LEADER 规则）。
	ErrNotConfigured = errors.New("flow: capability not configured")
	// ErrExpression 求值失败（表达式语法/运行错误，引擎按 fail-closed 处理）。
	ErrExpression = errors.New("flow: expression error")
	// ErrScript 脚本任务执行失败。
	ErrScript = errors.New("flow: script error")
)
