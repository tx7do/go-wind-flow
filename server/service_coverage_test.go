package flowserver

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	paginationv1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	flowv1 "github.com/tx7do/go-wind-flow/api/gen/go/flow/v1"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	"github.com/tx7do/go-wind-flow/store/memory"
)

// ============================================================
// 定义部署 / 实例查询 / 终止
// ============================================================

func TestDeployAndGetInstanceRPCs(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()

	def, err := svc.DeployDefinition(ctx, &flowv1.DeployDefinitionRequest{
		TenantId: "t1", Key: "leave", Name: "请假", Data: mustExampleBPMN(t),
	})
	if err != nil {
		t.Fatalf("DeployDefinition: %v", err)
	}
	if def.GetVersion() != 2 {
		t.Errorf("version = %d, want 2 (升版)", def.GetVersion())
	}
	if def.GetName() != "请假" {
		t.Errorf("name = %q", def.GetName())
	}
	// 非法定义（缺 process）→ InvalidArgument。
	_, err = svc.DeployDefinition(ctx, &flowv1.DeployDefinitionRequest{
		TenantId: "t1", Key: "bad", Format: "bpmn-xml",
		Data: []byte(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"/>`),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("bad definition code = %v, want InvalidArgument", status.Code(err))
	}

	inst, err := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
		TenantId: "t1", Key: "leave", StarterId: "u_emp",
	})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	got, err := svc.GetInstance(ctx, &flowv1.GetInstanceRequest{Id: inst.GetInstance().GetId()})
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.GetStatus() != flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_RUNNING {
		t.Errorf("status = %v", got.GetStatus())
	}
	if _, err := svc.GetInstance(ctx, &flowv1.GetInstanceRequest{Id: "ghost"}); status.Code(err) != codes.NotFound {
		t.Errorf("ghost instance code = %v, want NotFound", status.Code(err))
	}
}

func TestAbortInstanceRPC(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()
	started, err := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
		TenantId: "t1", Key: "leave", StarterId: "u_emp",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	id := started.GetInstance().GetId()
	out, err := svc.AbortInstance(ctx, &flowv1.AbortInstanceRequest{Id: id, ActorId: "admin"})
	if err != nil {
		t.Fatalf("AbortInstance: %v", err)
	}
	if out.GetStatus() != flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_ABORTED {
		t.Errorf("status = %v, want ABORTED", out.GetStatus())
	}
	// 再次终止 → FailedPrecondition。
	if _, err := svc.AbortInstance(ctx, &flowv1.AbortInstanceRequest{Id: id, ActorId: "admin"}); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("re-abort code = %v, want FailedPrecondition", status.Code(err))
	}
	// 不存在 → NotFound。
	if _, err := svc.AbortInstance(ctx, &flowv1.AbortInstanceRequest{Id: "ghost"}); status.Code(err) != codes.NotFound {
		t.Errorf("abort ghost code = %v, want NotFound", status.Code(err))
	}
}

func TestListInstancesFilters(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()

	for _, bk := range []string{"biz-1", "biz-2"} {
		if _, err := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
			TenantId: "t1", Key: "leave", StarterId: "u_emp", BusinessKey: bk,
		}); err != nil {
			t.Fatalf("start %s: %v", bk, err)
		}
	}

	all, err := svc.ListInstances(ctx, &flowv1.ListInstancesRequest{TenantId: "t1"})
	if err != nil || all.GetTotal() != 2 {
		t.Fatalf("all = %d, %v", all.GetTotal(), err)
	}

	running := flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_RUNNING
	got, err := svc.ListInstances(ctx, &flowv1.ListInstancesRequest{
		TenantId: "t1", Status: &running,
	})
	if err != nil || got.GetTotal() != 2 {
		t.Fatalf("running filter = %d, %v", got.GetTotal(), err)
	}
	approved := flowv1.FlowInstanceStatus_FLOW_INSTANCE_STATUS_APPROVED
	got, err = svc.ListInstances(ctx, &flowv1.ListInstancesRequest{
		TenantId: "t1", Status: &approved,
	})
	if err != nil || got.GetTotal() != 0 {
		t.Fatalf("approved filter = %d, %v", got.GetTotal(), err)
	}
	got, err = svc.ListInstances(ctx, &flowv1.ListInstancesRequest{
		TenantId: "t1", DefinitionKey: "leave", BusinessKey: "biz-2",
	})
	if err != nil || got.GetTotal() != 1 {
		t.Fatalf("business key filter = %d, %v", got.GetTotal(), err)
	}
	got, err = svc.ListInstances(ctx, &flowv1.ListInstancesRequest{
		TenantId: "t1", BusinessKey: "nope",
	})
	if err != nil || got.GetTotal() != 0 {
		t.Fatalf("no-match filter = %d, %v", got.GetTotal(), err)
	}

	// 分页：pageSize=1 只回 1 条，total 仍为全量。
	got, err = svc.ListInstances(ctx, &flowv1.ListInstancesRequest{
		TenantId: "t1", Paging: pageReq(1, 1),
	})
	if err != nil || got.GetTotal() != 2 || len(got.GetItems()) != 1 {
		t.Fatalf("paged = %d/%d, %v", got.GetTotal(), len(got.GetItems()), err)
	}
	// 越界页返回空列表。
	got, err = svc.ListInstances(ctx, &flowv1.ListInstancesRequest{
		TenantId: "t1", Paging: pageReq(9, 1),
	})
	if err != nil || got.GetTotal() != 2 || len(got.GetItems()) != 0 {
		t.Fatalf("overflow page = %d/%d, %v", got.GetTotal(), len(got.GetItems()), err)
	}
}

// ============================================================
// 任务：Get / Claim / Unclaim / ListCandidate
// ============================================================

// groupIdentity 组身份：bob ∈ finance。
type groupIdentity struct{ staticIdentity }

func (groupIdentity) ResolveGroupsForUser(_ context.Context, _, userID string) ([]string, error) {
	if userID == "bob" {
		return []string{"finance"}, nil
	}
	return nil, nil
}

func newGroupServices(t *testing.T) *Services {
	t.Helper()
	st := memory.New()
	eng := flow.New(
		flow.WithStore(st),
		flow.WithIdentity(groupIdentity{}),
		flow.WithImporters(bpmn.Importer{}),
		flow.WithExporters(bpmn.Exporter{}),
	)
	return New(eng, st)
}

func TestTaskGroupRPCs(t *testing.T) {
	svc := newGroupServices(t)
	ctx := context.Background()

	// 部署一个候选组任务流程（组身份解析为 bob ∈ finance）。
	groupXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="grp" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="t" candidateGroups="finance"/>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="t"/>
    <sequenceFlow id="f2" sourceRef="t" targetRef="e"/>
  </process>
</definitions>`)
	if _, err := svc.DeployDefinition(ctx, &flowv1.DeployDefinitionRequest{
		TenantId: "t1", Key: "grp", Data: groupXML,
	}); err != nil {
		t.Fatalf("deploy group flow: %v", err)
	}
	started, err := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
		TenantId: "t1", Key: "grp", StarterId: "u0",
	})
	if err != nil {
		t.Fatalf("start group flow: %v", err)
	}
	if len(started.GetTaskIds()) != 1 {
		t.Fatalf("tasks = %v, want 1", started.GetTaskIds())
	}
	taskID := started.GetTaskIds()[0]

	task, err := svc.GetTask(ctx, &flowv1.GetTaskRequest{Id: taskID})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.GetAssignee() != "" {
		t.Errorf("assignee = %q, want empty (候选组)", task.GetAssignee())
	}
	if _, err := svc.GetTask(ctx, &flowv1.GetTaskRequest{Id: "ghost"}); status.Code(err) != codes.NotFound {
		t.Errorf("get ghost task code = %v", status.Code(err))
	}

	// 候选组外用户认领 → PermissionDenied。
	if _, err := svc.ClaimTask(ctx, &flowv1.ClaimTaskRequest{TaskId: taskID, UserId: "eve"}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("claim by outsider code = %v, want PermissionDenied", status.Code(err))
	}
	// 组员认领成功。
	claimed, err := svc.ClaimTask(ctx, &flowv1.ClaimTaskRequest{TaskId: taskID, UserId: "bob"})
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	if claimed.GetAssignee() != "bob" {
		t.Errorf("claimed assignee = %q", claimed.GetAssignee())
	}
	// 退认。
	released, err := svc.UnclaimTask(ctx, &flowv1.UnclaimTaskRequest{TaskId: taskID, ActorId: "bob"})
	if err != nil {
		t.Fatalf("UnclaimTask: %v", err)
	}
	if released.GetAssignee() != "" {
		t.Errorf("released assignee = %q, want empty", released.GetAssignee())
	}
	// 候选列表可见。
	cand, err := svc.ListCandidateTasks(ctx, &flowv1.ListCandidateTasksRequest{
		TenantId: "t1", UserId: "bob",
	})
	if err != nil || cand.GetTotal() != 1 {
		t.Fatalf("ListCandidateTasks = %d, %v", cand.GetTotal(), err)
	}
	if started.GetInstance().GetId() == "" {
		t.Error("instance id empty")
	}
}

// ============================================================
// 定时器 / 消息 / 到期扫描
// ============================================================

func TestCheckDueTasksRPC(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()
	if _, err := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
		TenantId: "t1", Key: "leave", StarterId: "u_emp",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	missing := &flowv1.CheckDueTasksRequest{}
	out, err := svc.CheckDueTasks(ctx, missing)
	if err != nil {
		t.Fatalf("CheckDueTasks: %v", err)
	}
	if out.GetTotal() != 0 {
		t.Errorf("due = %d, want 0（任务无 due）", out.GetTotal())
	}
	// 显式 asOf 走另一条分支。
	future := time.Now().Add(24 * time.Hour)
	out, err = svc.CheckDueTasks(ctx, &flowv1.CheckDueTasksRequest{
		AsOf: timestamppbOf(future),
	})
	if err != nil || out.GetTotal() != 0 {
		t.Fatalf("CheckDueTasks(asOf) = %d, %v", out.GetTotal(), err)
	}
}

func TestResumeDueTimersRPC(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()
	out, err := svc.ResumeDueTimers(ctx, &flowv1.ResumeDueTimersRequest{})
	if err != nil {
		t.Fatalf("ResumeDueTimers: %v", err)
	}
	if len(out.GetTokenIds()) != 0 {
		t.Errorf("resumed = %v, want none", out.GetTokenIds())
	}
}

func TestSendMessageRPC(t *testing.T) {
	svc := newServices(t)
	ctx := context.Background()
	started, err := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
		TenantId: "t1", Key: "leave", StarterId: "u_emp",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	resp, err := svc.SendMessage(ctx, &flowv1.SendMessageRequest{
		InstanceId: started.GetInstance().GetId(), Message: "not-waiting",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.GetDelivered() {
		t.Error("delivered = true, want false（无等待令牌）")
	}
}

// ============================================================
// 注册与错误映射
// ============================================================

type fakeRegistrar struct {
	names []string
}

func (f *fakeRegistrar) RegisterService(desc *grpc.ServiceDesc, _ any) {
	f.names = append(f.names, desc.ServiceName)
}

func TestRegisterAll(t *testing.T) {
	svc := newServices(t)
	reg := &fakeRegistrar{}
	svc.RegisterAll(reg)
	if len(reg.names) != 4 {
		t.Fatalf("registered %d services, want 4: %v", len(reg.names), reg.names)
	}
}

func TestMapErrorCodes(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{flowErrNotFound(), codes.NotFound},
		{flowErrForbidden(), codes.PermissionDenied},
		{flowErrInvalidState(), codes.FailedPrecondition},
		{flowErrInvalidDefinition(), codes.InvalidArgument},
		{flowErrExpression(), codes.FailedPrecondition},
		{flowErrNotConfigured(), codes.Internal},
	}
	for _, tc := range cases {
		if got := status.Code(mapError(tc.err)); got != tc.want {
			t.Errorf("mapError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
	if mapError(nil) != nil {
		t.Error("mapError(nil) should be nil")
	}
	// 未知错误原样返回（不包装成 gRPC 状态）。
	plain := context.Canceled
	if got := mapError(plain); got != plain {
		t.Errorf("unknown error should pass through, got %v", got)
	}
}

func TestNilStoreTxPassthrough(t *testing.T) {
	// store 为 nil 时写路径直连引擎，不包事务。
	st := newServices(t)
	eng := st.engine
	svc := New(eng, nil)
	ctx := context.Background()
	if _, err := svc.StartInstance(ctx, &flowv1.StartInstanceRequest{
		TenantId: "t1", Key: "leave", StarterId: "u_emp",
	}); err != nil {
		t.Fatalf("start with nil store: %v", err)
	}
}

func mustExampleBPMN(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../examples/leave-approval.bpmn.xml")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	return data
}

func timestamppbOf(tm time.Time) *timestamppb.Timestamp { return timestamppb.New(tm) }

func pageReq(page, size uint32) *paginationv1.PagingRequest {
	return &paginationv1.PagingRequest{Page: &page, PageSize: &size}
}

func flowErrNotFound() error          { return fmt.Errorf("x: %w", flow.ErrNotFound) }
func flowErrForbidden() error         { return fmt.Errorf("x: %w", flow.ErrForbidden) }
func flowErrInvalidState() error      { return fmt.Errorf("x: %w", flow.ErrInvalidState) }
func flowErrInvalidDefinition() error { return fmt.Errorf("x: %w", flow.ErrInvalidDefinition) }
func flowErrExpression() error        { return fmt.Errorf("x: %w", flow.ErrExpression) }
func flowErrNotConfigured() error     { return fmt.Errorf("x: %w", flow.ErrNotConfigured) }
