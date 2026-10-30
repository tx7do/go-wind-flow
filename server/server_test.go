package flowserver

import (
	"context"
	"os"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	flowv1 "github.com/tx7do/go-wind-flow/api/gen/go/flow/v1"

	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"

	"github.com/tx7do/go-wind-flow/flow"
	"github.com/tx7do/go-wind-flow/store/memory"
)

// staticIdentity 测试身份解析：LEADER → boss。
type staticIdentity struct{}

func (staticIdentity) ResolveApprovers(_ context.Context, req flow.ResolveRequest) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, r := range req.Rules {
		v := r.Value
		if r.Kind == flow.ApproverStarter {
			v = req.StarterID
		}
		if r.Kind == flow.ApproverLeader {
			v = "boss"
		}
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out, nil
}

func (staticIdentity) ResolveDelegate(_ context.Context, _, userID string) (string, error) {
	return userID, nil
}

func newServices(t *testing.T) *Services {
	t.Helper()
	st := memory.New() // 引擎与服务共享同一存储
	eng := flow.New(
		flow.WithStore(st),
		flow.WithIdentity(staticIdentity{}),
		flow.WithImporters(bpmn.Importer{}),
		flow.WithExporters(bpmn.Exporter{}),
	)
	data, err := os.ReadFile("../examples/leave-approval.bpmn.xml")
	if err != nil {
		t.Fatalf("read example bpmn: %v", err)
	}
	if _, err := eng.Deploy(context.Background(), flow.DeployRequest{
		TenantID: "t1", Key: "leave", Data: data,
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	return New(eng, st)
}

func TestDefinitionServiceRPCs(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()

	def, err := svc.GetDefinition(ctx, &flowv1.GetDefinitionRequest{TenantId: "t1", Key: "leave"})
	if err != nil {
		t.Fatalf("GetDefinition: %v", err)
	}
	if def.GetKey() != "leave" || def.GetVersion() != 1 || def.GetFormat() != "bpmn-xml" {
		t.Fatalf("definition proto mapping broken: %+v", def)
	}

	list, err := svc.ListDefinitions(ctx, &flowv1.ListDefinitionsRequest{TenantId: "t1"})
	if err != nil || list.GetTotal() != 1 || len(list.GetItems()) != 1 {
		t.Fatalf("ListDefinitions = %d/%v", list.GetTotal(), err)
	}

	// 不存在的定义 → NotFound。
	_, err = svc.GetDefinition(ctx, &flowv1.GetDefinitionRequest{TenantId: "t1", Key: "nope"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("missing definition code = %v, want NotFound", status.Code(err))
	}

	exp, err := svc.ExportDefinition(ctx, &flowv1.ExportDefinitionRequest{
		TenantId: "t1", Key: "leave", Format: "bpmn-xml",
	})
	if err != nil || len(exp.GetData()) == 0 {
		t.Fatalf("ExportDefinition = %d bytes, %v", len(exp.GetData()), err)
	}
}

func TestInstanceTaskLifecycleRPCs(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()

	vars, _ := structpb.NewStruct(map[string]any{"days": 5})
	started, err := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
		TenantId:  "t1",
		Key:       "leave",
		StarterId: "u_emp",
		Variables: vars,
	})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	if started.GetInstance().GetStatus() != flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_RUNNING {
		t.Fatalf("status = %s, want RUNNING", started.GetInstance().GetStatus())
	}
	if len(started.GetTaskIds()) != 1 {
		t.Fatalf("task_ids = %d, want 1", len(started.GetTaskIds()))
	}

	// 非办理人办结 → PermissionDenied。
	_, err = svc.CompleteTask(ctx, &flowv1.CompleteTaskRequest{
		TaskId: started.GetTaskIds()[0], ActorId: "not_boss", Outcome: flowv1.FlowTaskOutcome_FLOW_TASK_OUTCOME_APPROVE,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("non-assignee code = %v, want PermissionDenied", status.Code(err))
	}

	// 办理人通过 → days>3 进入总监审批。
	_, err = svc.CompleteTask(ctx, &flowv1.CompleteTaskRequest{
		TaskId: started.GetTaskIds()[0], ActorId: "boss", Outcome: flowv1.FlowTaskOutcome_FLOW_TASK_OUTCOME_APPROVE,
	})
	if err != nil {
		t.Fatalf("CompleteTask(boss): %v", err)
	}

	pending, err := svc.ListPendingTasks(ctx, &flowv1.ListPendingTasksRequest{TenantId: "t1", Assignee: "u_1001"})
	if err != nil || pending.GetTotal() != 1 {
		t.Fatalf("ListPendingTasks = %d, %v", pending.GetTotal(), err)
	}

	// 总监通过 → 实例 APPROVED。
	done, err := svc.CompleteTask(ctx, &flowv1.CompleteTaskRequest{
		TaskId: pending.GetItems()[0].GetId(), ActorId: "u_1001", Outcome: flowv1.FlowTaskOutcome_FLOW_TASK_OUTCOME_APPROVE,
	})
	if err != nil {
		t.Fatalf("CompleteTask(director): %v", err)
	}
	if done.GetInstance().GetStatus() != flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_APPROVED {
		t.Fatalf("status = %s, want APPROVED", done.GetInstance().GetStatus())
	}

	// 实例查询（状态过滤）。
	running, err := svc.ListInstances(ctx, &flowv1.ListInstancesRequest{
		TenantId: "t1", Status: flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_RUNNING.Enum(),
	})
	if err != nil || running.GetTotal() != 0 {
		t.Fatalf("ListInstances(RUNNING) = %d, %v", running.GetTotal(), err)
	}

	// 任务查询 + 历史。
	tasks, err := svc.ListInstanceTasks(ctx, &flowv1.ListInstanceTasksRequest{InstanceId: started.GetInstance().GetId()})
	if err != nil || tasks.GetTotal() != 2 {
		t.Fatalf("ListInstanceTasks = %d, %v", tasks.GetTotal(), err)
	}
	hist, err := svc.ListInstanceHistory(ctx, &flowv1.ListInstanceHistoryRequest{InstanceId: started.GetInstance().GetId()})
	if err != nil || len(hist.GetItems()) == 0 {
		t.Fatalf("history empty: %v", err)
	}
	for _, ev := range hist.GetItems() {
		if ev.GetType() == flowv1.FlowEventType_FLOW_EVENT_TYPE_PROCESS_ENDED {
			return // 终结事件在，映射正确
		}
	}
	t.Fatal("PROCESS_ENDED event missing in history")
}

func TestWithdrawAndForwardRPCs(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()

	started, _ := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
		TenantId: "t1", Key: "leave", StarterId: "u_emp",
	})

	// 转办给他人后原办理人失去办结权。
	fwd, err := svc.ForwardTask(ctx, &flowv1.ForwardTaskRequest{
		TaskId: started.GetTaskIds()[0], ActorId: "boss", ToUserId: "boss2",
	})
	if err != nil || fwd.GetAssignee() != "boss2" {
		t.Fatalf("ForwardTask = %+v, %v", fwd, err)
	}

	// 非发起人撤回 → PermissionDenied。
	_, err = svc.WithdrawInstance(ctx, &flowv1.WithdrawInstanceRequest{Id: started.GetInstance().GetId(), ActorId: "whoever"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("non-starter withdraw code = %v, want PermissionDenied", status.Code(err))
	}

	withdrawn, err := svc.WithdrawInstance(ctx, &flowv1.WithdrawInstanceRequest{
		Id: started.GetInstance().GetId(), ActorId: "u_emp",
	})
	if err != nil || withdrawn.GetStatus() != flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_WITHDRAWN {
		t.Fatalf("withdraw = %s, %v", withdrawn.GetStatus(), err)
	}

	// 终结后重复撤回 → FailedPrecondition。
	_, err = svc.WithdrawInstance(ctx, &flowv1.WithdrawInstanceRequest{Id: started.GetInstance().GetId(), ActorId: "u_emp"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("double withdraw code = %v, want FailedPrecondition", status.Code(err))
	}
}
