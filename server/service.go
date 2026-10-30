// Package flowserver 基于 flow.Engine 实现 api 子模块定义的四个 gRPC 服务
// （FlowDefinition / FlowInstance / FlowTask / FlowHistory），负责
// proto ↔ 领域结构映射、错误码映射与 WithTx 事务接线。
//
// 纯 gRPC：宿主用任意 grpc.ServiceRegistrar（kratos grpc server、裸 grpc.Server）
// 注册——flowv1.RegisterXxxServiceServer，或直接 svcs.RegisterAll(registrar)。
// HTTP/REST 由宿主 BFF 层定义（家族惯例），本包不做 HTTP。
//
// 事务：构造时传入 WithTx 能力的 Store（store/ent、store/gorm），写路径 RPC
// 自动包事务；传 nil 则直连引擎（嵌入式场景）。
package flowserver

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	flowv1 "github.com/tx7do/go-wind-flow/api/gen/go/flow/v1"

	"github.com/tx7do/go-wind-flow/flow"
)

// Store 在 flow.Store 之上要求 WithTx 事务能力（store/ent、store/gorm 均满足）。
type Store interface {
	flow.Store
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Services 四个流程服务共用一个实现体。
// 嵌入 Unimplemented 以满足 protoc-gen-go-grpc 的前向兼容要求；
// 宿主按需注册其一或全部。
type Services struct {
	flowv1.UnimplementedFlowDefinitionServiceServer
	flowv1.UnimplementedFlowInstanceServiceServer
	flowv1.UnimplementedFlowTaskServiceServer
	flowv1.UnimplementedFlowHistoryServiceServer

	engine *flow.Engine
	store  Store // 必填：查询与写路径都要用；嵌入式场景用 memory.New()
}

// New 构造服务。store 提供查询与 WithTx 事务能力（store/ent、store/gorm、
// store/memory 均满足接口）；写路径 RPC 自动包事务。
func New(engine *flow.Engine, store Store) *Services {
	return &Services{engine: engine, store: store}
}

var (
	_ flowv1.FlowDefinitionServiceServer = (*Services)(nil)
	_ flowv1.FlowInstanceServiceServer   = (*Services)(nil)
	_ flowv1.FlowTaskServiceServer       = (*Services)(nil)
	_ flowv1.FlowHistoryServiceServer    = (*Services)(nil)
)

// RegisterAll 把四个服务注册到任意 grpc.ServiceRegistrar。
func (s *Services) RegisterAll(reg grpc.ServiceRegistrar) {
	flowv1.RegisterFlowDefinitionServiceServer(reg, s)
	flowv1.RegisterFlowInstanceServiceServer(reg, s)
	flowv1.RegisterFlowTaskServiceServer(reg, s)
	flowv1.RegisterFlowHistoryServiceServer(reg, s)
}

// tx 写路径事务包裹：store 为 nil 时直跑 fn。
func (s *Services) tx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.store == nil {
		return fn(ctx)
	}
	return s.store.WithTx(ctx, fn)
}

// mapError 引擎哨兵错误 → gRPC 状态码。
func mapError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, flow.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, flow.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, flow.ErrInvalidState):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, flow.ErrInvalidDefinition):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, flow.ErrExpression), errors.Is(err, flow.ErrScript):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, flow.ErrNotConfigured):
		return status.Error(codes.Internal, err.Error())
	default:
		return err
	}
}
