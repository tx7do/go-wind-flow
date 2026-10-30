// Package api 是 go-wind-flow 的 Protobuf 服务契约模块。
//
// 目录：
//
//	protos/flow/v1/  四服务源文件（definition / instance / task / history），
//	                 纯 gRPC 契约——无 HTTP 注解，HTTP 路由由宿主 BFF 层定义
//	gen/go/flow/v1/  生成码（go / gRPC 两件套，已入库；make proto 重新生成）
//
// 同源 proto 发布在 BSR：buf.build/go-wind/go-wind-flow（label 与 git tag 对齐），
// 非 Go 消费方（TS / Dart 等）与跨仓 proto 引用直接依赖 BSR 模块。
package api
