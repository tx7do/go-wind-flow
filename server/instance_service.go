package flowserver

import (
	"context"
	"time"

	flowv1 "github.com/tx7do/go-wind-flow/api/gen/go/flow/v1"

	"github.com/tx7do/go-wind-flow/flow"
)

// ============================================================
// FlowDefinitionService
// ============================================================

func (s *Services) DeployDefinition(ctx context.Context, req *flowv1.DeployDefinitionRequest) (*flowv1.FlowDefinition, error) {
	def, err := s.engine.Deploy(ctx, flow.DeployRequest{
		TenantID: req.GetTenantId(),
		Key:      req.GetKey(),
		Name:     req.GetName(),
		Format:   req.GetFormat(),
		Data:     req.GetData(),
	})
	if err != nil {
		return nil, mapError(err)
	}
	return defToProto(def), nil
}

func (s *Services) GetDefinition(ctx context.Context, req *flowv1.GetDefinitionRequest) (*flowv1.FlowDefinition, error) {
	def, err := s.engine.GetDefinition(ctx, req.GetTenantId(), req.GetKey(), int(req.GetVersion()))
	if err != nil {
		return nil, mapError(err)
	}
	return defToProto(def), nil
}

func (s *Services) ListDefinitions(ctx context.Context, req *flowv1.ListDefinitionsRequest) (*flowv1.ListDefinitionsResponse, error) {
	defs, err := s.store.Definitions().List(ctx, req.GetTenantId(), "")
	if err != nil {
		return nil, mapError(err)
	}
	items, total := paginate(defs, req.GetPaging())
	out := &flowv1.ListDefinitionsResponse{Total: total}
	for _, d := range items {
		out.Items = append(out.Items, defToProto(d))
	}
	return out, nil
}

func (s *Services) ExportDefinition(ctx context.Context, req *flowv1.ExportDefinitionRequest) (*flowv1.ExportDefinitionResponse, error) {
	data, err := s.engine.ExportDefinition(ctx, req.GetTenantId(), req.GetKey(), int(req.GetVersion()), req.GetFormat())
	if err != nil {
		return nil, mapError(err)
	}
	return &flowv1.ExportDefinitionResponse{Data: data, Format: req.GetFormat()}, nil
}

// ============================================================
// FlowInstanceService
// ============================================================

func (s *Services) StartInstance(ctx context.Context, req *flowv1.StartInstanceRequest) (*flowv1.StartInstanceResponse, error) {
	var (
		inst    *flow.Instance
		taskIDs []string
		err     error
	)
	err = s.tx(ctx, func(ctx context.Context) error {
		inst, taskIDs, err = s.engine.Start(ctx, flow.StartRequest{
			TenantID:     req.GetTenantId(),
			Key:          req.GetKey(),
			Version:      int(req.GetVersion()),
			BusinessKey:  req.GetBusinessKey(),
			BusinessType: req.GetBusinessType(),
			StarterID:    req.GetStarterId(),
			Variables:    structToVars(req.GetVariables()),
		})
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	pb, err := instToProto(inst)
	if err != nil {
		return nil, mapError(err)
	}
	return &flowv1.StartInstanceResponse{Instance: pb, TaskIds: taskIDs}, nil
}

func (s *Services) GetInstance(ctx context.Context, req *flowv1.GetInstanceRequest) (*flowv1.FlowInstance, error) {
	inst, err := s.engine.GetInstance(ctx, req.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	return instToProto(inst)
}

func (s *Services) ListInstances(ctx context.Context, req *flowv1.ListInstancesRequest) (*flowv1.ListInstancesResponse, error) {
	insts, err := s.store.Instances().List(ctx, req.GetTenantId())
	if err != nil {
		return nil, mapError(err)
	}

	// 过滤（可选条件全命中才保留）。
	var filtered []*flow.Instance
	for _, inst := range insts {
		if req.Status != nil && instStatusToDomain(*req.Status) != inst.Status {
			continue
		}
		if req.GetDefinitionKey() != "" && inst.DefinitionKey != req.GetDefinitionKey() {
			continue
		}
		if req.GetBusinessKey() != "" && inst.BusinessKey != req.GetBusinessKey() {
			continue
		}
		filtered = append(filtered, inst)
	}

	items, total := paginate(filtered, req.GetPaging())
	out := &flowv1.ListInstancesResponse{Total: total}
	for _, inst := range items {
		pb, err := instToProto(inst)
		if err != nil {
			return nil, mapError(err)
		}
		out.Items = append(out.Items, pb)
	}
	return out, nil
}

func (s *Services) WithdrawInstance(ctx context.Context, req *flowv1.WithdrawInstanceRequest) (*flowv1.FlowInstance, error) {
	err := s.tx(ctx, func(ctx context.Context) error {
		return s.engine.Withdraw(ctx, req.GetId(), req.GetActorId())
	})
	if err != nil {
		return nil, mapError(err)
	}
	inst, err := s.engine.GetInstance(ctx, req.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	return instToProto(inst)
}

func (s *Services) AbortInstance(ctx context.Context, req *flowv1.AbortInstanceRequest) (*flowv1.FlowInstance, error) {
	err := s.tx(ctx, func(ctx context.Context) error {
		return s.engine.Abort(ctx, req.GetId(), req.GetActorId())
	})
	if err != nil {
		return nil, mapError(err)
	}
	inst, err := s.engine.GetInstance(ctx, req.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	return instToProto(inst)
}

// ============================================================
// FlowTaskService
// ============================================================

func (s *Services) CompleteTask(ctx context.Context, req *flowv1.CompleteTaskRequest) (*flowv1.CompleteTaskResponse, error) {
	// 先取任务定位实例，办结后返回实例最新状态。
	task, err := s.store.Tasks().Get(ctx, req.GetTaskId())
	if err != nil {
		return nil, mapError(err)
	}
	err = s.tx(ctx, func(ctx context.Context) error {
		return s.engine.CompleteTask(ctx, flow.CompleteTaskRequest{
			TaskID:    req.GetTaskId(),
			ActorID:   req.GetActorId(),
			Outcome:   outcomeToDomain(req.GetOutcome()),
			Comment:   req.GetComment(),
			Variables: structToVars(req.GetVariables()),
		})
	})
	if err != nil {
		return nil, mapError(err)
	}
	inst, err := s.engine.GetInstance(ctx, task.InstanceID)
	if err != nil {
		return nil, mapError(err)
	}
	pb, err := instToProto(inst)
	if err != nil {
		return nil, mapError(err)
	}
	return &flowv1.CompleteTaskResponse{Instance: pb}, nil
}

func (s *Services) ForwardTask(ctx context.Context, req *flowv1.ForwardTaskRequest) (*flowv1.FlowTask, error) {
	err := s.tx(ctx, func(ctx context.Context) error {
		return s.engine.ForwardTask(ctx, req.GetTaskId(), req.GetActorId(), req.GetToUserId())
	})
	if err != nil {
		return nil, mapError(err)
	}
	task, err := s.store.Tasks().Get(ctx, req.GetTaskId())
	if err != nil {
		return nil, mapError(err)
	}
	return taskToProto(task), nil
}

func (s *Services) GetTask(ctx context.Context, req *flowv1.GetTaskRequest) (*flowv1.FlowTask, error) {
	task, err := s.store.Tasks().Get(ctx, req.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	return taskToProto(task), nil
}

func (s *Services) ListPendingTasks(ctx context.Context, req *flowv1.ListPendingTasksRequest) (*flowv1.ListTasksResponse, error) {
	tasks, err := s.engine.ListPendingTasks(ctx, req.GetTenantId(), req.GetAssignee())
	if err != nil {
		return nil, mapError(err)
	}
	items, total := paginate(tasks, req.GetPaging())
	out := &flowv1.ListTasksResponse{Total: total}
	for _, t := range items {
		out.Items = append(out.Items, taskToProto(t))
	}
	return out, nil
}

func (s *Services) ListInstanceTasks(ctx context.Context, req *flowv1.ListInstanceTasksRequest) (*flowv1.ListTasksResponse, error) {
	tasks, err := s.engine.ListInstanceTasks(ctx, req.GetInstanceId())
	if err != nil {
		return nil, mapError(err)
	}
	out := &flowv1.ListTasksResponse{Total: int64(len(tasks))}
	for _, t := range tasks {
		out.Items = append(out.Items, taskToProto(t))
	}
	return out, nil
}

func (s *Services) CheckDueTasks(ctx context.Context, req *flowv1.CheckDueTasksRequest) (*flowv1.ListTasksResponse, error) {
	now := time.Now()
	if req.GetAsOf() != nil {
		now = req.GetAsOf().AsTime()
	}
	due, err := s.engine.CheckDueTasks(ctx, now)
	if err != nil {
		return nil, mapError(err)
	}
	out := &flowv1.ListTasksResponse{Total: int64(len(due))}
	for _, t := range due {
		out.Items = append(out.Items, taskToProto(t))
	}
	return out, nil
}

// ============================================================
// FlowHistoryService
// ============================================================

func (s *Services) ListInstanceHistory(ctx context.Context, req *flowv1.ListInstanceHistoryRequest) (*flowv1.ListInstanceHistoryResponse, error) {
	events, err := s.engine.ListHistory(ctx, req.GetInstanceId())
	if err != nil {
		return nil, mapError(err)
	}
	out := &flowv1.ListInstanceHistoryResponse{}
	for _, e := range events {
		out.Items = append(out.Items, histToProto(e))
	}
	return out, nil
}
