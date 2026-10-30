package flowserver

import (
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	paginationv1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	flowv1 "github.com/tx7do/go-wind-flow/api/gen/go/flow/v1"

	"github.com/tx7do/go-wind-flow/flow"
)

// ============================================================
// 变量环境
// ============================================================

func varsToStruct(v flow.Vars) (*structpb.Struct, error) {
	if v == nil {
		return structpb.NewStruct(nil)
	}
	return structpb.NewStruct(map[string]any(v))
}

func structToVars(s *structpb.Struct) flow.Vars {
	if s == nil {
		return flow.Vars{}
	}
	return flow.Vars(s.AsMap())
}

// ============================================================
// 实体 proto 化
// ============================================================

func defToProto(d *flow.Definition) *flowv1.FlowDefinition {
	return &flowv1.FlowDefinition{
		Key:       d.Key,
		Name:      d.Name,
		Version:   int32(d.Version),
		TenantId:  d.TenantID,
		Format:    d.Format,
		Source:    string(d.Source),
		CreatedAt: timestamppb.New(d.CreatedAt),
	}
}

func instToProto(i *flow.Instance) (*flowv1.FlowInstance, error) {
	vars, err := varsToStruct(i.Variables)
	if err != nil {
		return nil, err
	}
	var endedAt *timestamppb.Timestamp
	if i.EndedAt != nil {
		endedAt = timestamppb.New(*i.EndedAt)
	}
	return &flowv1.FlowInstance{
		Id:                i.ID,
		TenantId:          i.TenantID,
		DefinitionKey:     i.DefinitionKey,
		DefinitionVersion: int32(i.DefinitionVersion),
		BusinessKey:       i.BusinessKey,
		BusinessType:      i.BusinessType,
		StarterId:         i.StarterID,
		Status:            instStatusToProto(i.Status),
		Variables:         vars,
		ParentInstanceId:  i.ParentInstanceID,
		ParentTokenId:     i.ParentTokenID,
		CreatedAt:         timestamppb.New(i.CreatedAt),
		EndedAt:           endedAt,
	}, nil
}

func taskToProto(t *flow.Task) *flowv1.FlowTask {
	var dueAt, endedAt *timestamppb.Timestamp
	if t.DueAt != nil {
		dueAt = timestamppb.New(*t.DueAt)
	}
	if t.EndedAt != nil {
		endedAt = timestamppb.New(*t.EndedAt)
	}
	return &flowv1.FlowTask{
		Id:         t.ID,
		InstanceId: t.InstanceID,
		TenantId:   t.TenantID,
		NodeId:     t.NodeID,
		TokenId:    t.TokenID,
		Name:       t.Name,
		Assignee:   t.Assignee,
		Status:     taskStatusToProto(t.Status),
		Outcome:    t.Outcome,
		Comment:    t.Comment,
		DueAt:      dueAt,
		CreatedAt:  timestamppb.New(t.CreatedAt),
		EndedAt:    endedAt,
	}
}

func histToProto(h *flow.HistoryEvent) *flowv1.FlowHistoryEvent {
	payload, _ := varsToStruct(h.Payload)
	return &flowv1.FlowHistoryEvent{
		Id:         h.ID,
		InstanceId: h.InstanceID,
		Type:       eventTypeToProto(h.Type),
		NodeId:     h.NodeID,
		TaskId:     h.TaskID,
		ActorId:    h.ActorID,
		Payload:    payload,
		CreatedAt:  timestamppb.New(h.CreatedAt),
	}
}

// ============================================================
// 枚举映射
// ============================================================

func instStatusToProto(s flow.InstanceStatus) flowv1.FlowInstanceStatus {
	switch s {
	case flow.InstanceRunning:
		return flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_RUNNING
	case flow.InstanceApproved:
		return flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_APPROVED
	case flow.InstanceRejected:
		return flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_REJECTED
	case flow.InstanceWithdrawn:
		return flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_WITHDRAWN
	case flow.InstanceAborted:
		return flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_ABORTED
	}
	return flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_UNSPECIFIED
}

func instStatusToDomain(s flowv1.FlowInstanceStatus) flow.InstanceStatus {
	switch s {
	case flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_RUNNING:
		return flow.InstanceRunning
	case flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_APPROVED:
		return flow.InstanceApproved
	case flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_REJECTED:
		return flow.InstanceRejected
	case flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_WITHDRAWN:
		return flow.InstanceWithdrawn
	case flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_ABORTED:
		return flow.InstanceAborted
	}
	return ""
}

func taskStatusToProto(s flow.TaskStatus) flowv1.FlowTaskStatus {
	switch s {
	case flow.TaskPending:
		return flowv1.FlowTaskStatus_FLOW_TASK_STATUS_PENDING
	case flow.TaskApproved:
		return flowv1.FlowTaskStatus_FLOW_TASK_STATUS_APPROVED
	case flow.TaskRejected:
		return flowv1.FlowTaskStatus_FLOW_TASK_STATUS_REJECTED
	case flow.TaskCancelled:
		return flowv1.FlowTaskStatus_FLOW_TASK_STATUS_CANCELLED
	case flow.TaskForwarded:
		return flowv1.FlowTaskStatus_FLOW_TASK_STATUS_FORWARDED
	}
	return flowv1.FlowTaskStatus_FLOW_TASK_STATUS_UNSPECIFIED
}

func outcomeToDomain(o flowv1.FlowTaskOutcome) string {
	if o == flowv1.FlowTaskOutcome_FLOW_TASK_OUTCOME_REJECT {
		return flow.OutcomeReject
	}
	return flow.OutcomeApprove
}

func eventTypeToProto(t flow.EventType) flowv1.FlowEventType {
	switch t {
	case flow.EventProcessStarted:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_PROCESS_STARTED
	case flow.EventProcessEnded:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_PROCESS_ENDED
	case flow.EventTokenMoved:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_TOKEN_MOVED
	case flow.EventGatewayDecided:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_GATEWAY_DECIDED
	case flow.EventTaskCreated:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_TASK_CREATED
	case flow.EventTaskCompleted:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_TASK_COMPLETED
	case flow.EventTaskForwarded:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_TASK_FORWARDED
	case flow.EventTaskDue:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_TASK_DUE
	case flow.EventTaskApproverAdded:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_TASK_APPROVER_ADDED
	case flow.EventInstanceReturned:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_INSTANCE_RETURNED
	case flow.EventTimerFired:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_TIMER_FIRED
	case flow.EventMessageReceived:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_MESSAGE_RECEIVED
	case flow.EventVariableChanged:
		return flowv1.FlowEventType_FLOW_EVENT_TYPE_VARIABLE_CHANGED
	}
	return flowv1.FlowEventType_FLOW_EVENT_TYPE_UNSPECIFIED
}

// ============================================================
// 分页（内存分页：定义/实例/任务均为可列举规模；深分页走存储层是 M1 后续项）
// ============================================================

func paginate[N any](items []N, p *paginationv1.PagingRequest) ([]N, int64) {
	total := int64(len(items))
	if p == nil || p.GetNoPaging() || p.GetPageSize() == 0 {
		return items, total
	}
	page := int(p.GetPage())
	if page <= 0 {
		page = 1
	}
	size := int(p.GetPageSize())
	start := (page - 1) * size
	if start >= len(items) {
		return []N{}, total
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total
}
