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
			TaskID:         req.GetTaskId(),
			ActorID:        req.GetActorId(),
			Outcome:        outcomeToDomain(req.GetOutcome()),
			Comment:        req.GetComment(),
			Variables:      structToVars(req.GetVariables()),
			LocalVariables: structToVars(req.GetLocalVariables()),
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

// ClaimTask 认领候选组任务（组员鉴权经引擎 IdentityProvider）。
func (s *Services) ClaimTask(ctx context.Context, req *flowv1.ClaimTaskRequest) (*flowv1.FlowTask, error) {
	if err := s.engine.ClaimTask(ctx, req.GetTaskId(), req.GetUserId()); err != nil {
		return nil, mapError(err)
	}
	task, err := s.store.Tasks().Get(ctx, req.GetTaskId())
	if err != nil {
		return nil, mapError(err)
	}
	return taskToProto(task), nil
}

// UnclaimTask 退认（放回待认领池）。
func (s *Services) UnclaimTask(ctx context.Context, req *flowv1.UnclaimTaskRequest) (*flowv1.FlowTask, error) {
	if err := s.engine.UnclaimTask(ctx, req.GetTaskId(), req.GetActorId()); err != nil {
		return nil, mapError(err)
	}
	task, err := s.store.Tasks().Get(ctx, req.GetTaskId())
	if err != nil {
		return nil, mapError(err)
	}
	return taskToProto(task), nil
}

// ListCandidateTasks 用户可认领的组待办。
func (s *Services) ListCandidateTasks(ctx context.Context, req *flowv1.ListCandidateTasksRequest) (*flowv1.ListTasksResponse, error) {
	tasks, err := s.engine.ListCandidateTasks(ctx, req.GetTenantId(), req.GetUserId())
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

func (s *Services) AddSigners(ctx context.Context, req *flowv1.AddSignersRequest) (*flowv1.ListTasksResponse, error) {
	created, err := s.engine.AddTaskApprovers(ctx, req.GetTaskId(), req.GetActorId(), req.GetUserIds())
	if err != nil {
		return nil, mapError(err)
	}
	out := &flowv1.ListTasksResponse{Total: int64(len(created))}
	for _, t := range created {
		out.Items = append(out.Items, taskToProto(t))
	}
	return out, nil
}

func (s *Services) ReturnTo(ctx context.Context, req *flowv1.ReturnToRequest) (*flowv1.CompleteTaskResponse, error) {
	task, err := s.store.Tasks().Get(ctx, req.GetTaskId())
	if err != nil {
		return nil, mapError(err)
	}
	err = s.tx(ctx, func(ctx context.Context) error {
		return s.engine.ReturnTo(ctx, req.GetTaskId(), req.GetActorId(), req.GetTargetNodeId(), req.GetComment())
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

// ResumeDueTimers 到期定时器扫描（宿主 cron 驱动）。
func (s *Services) ResumeDueTimers(ctx context.Context, req *flowv1.ResumeDueTimersRequest) (*flowv1.ResumeDueTimersResponse, error) {
	now := time.Now()
	if req.GetAsOf() != nil {
		now = req.GetAsOf().AsTime()
	}
	resumed, err := s.engine.ResumeDueTimers(ctx, now)
	if err != nil {
		return nil, mapError(err)
	}
	out := &flowv1.ResumeDueTimersResponse{}
	for _, tk := range resumed {
		out.TokenIds = append(out.TokenIds, tk.ID)
	}
	return out, nil
}

// SendMessage 消息投递（外部回调解锁消息事件）。
func (s *Services) SendMessage(ctx context.Context, req *flowv1.SendMessageRequest) (*flowv1.SendMessageResponse, error) {
	delivered, err := s.engine.SendMessage(ctx, req.GetInstanceId(), req.GetMessage(), structToVars(req.GetPayload()))
	if err != nil {
		return nil, mapError(err)
	}
	return &flowv1.SendMessageResponse{Delivered: delivered}, nil
}
