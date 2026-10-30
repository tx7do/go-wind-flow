package go_wind_flow_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
	"github.com/tx7do/go-wind-flow/store/memory"

	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	windjson "github.com/tx7do/go-wind-flow/format/windjson"
)

// ============================================================
// 测试替身
// ============================================================

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

type seqIDGen struct {
	mu sync.Mutex
	n  int
}

func (g *seqIDGen) next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return "id-" + strings.Repeat("0", 0) + itoa(g.n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type recLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (l *recLogger) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, format)
}

func (l *recLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.msgs)
}

type recListener struct {
	mu     sync.Mutex
	events []flow.EventType
}

func (l *recListener) OnFlowEvent(_ context.Context, ev *flow.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, ev.Type)
}

func (l *recListener) has(typ flow.EventType) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.events {
		if e == typ {
			return true
		}
	}
	return false
}

// groupIdentity 与 engine_group_test.go 的组身份语义一致（fin_a/fin_b ∈ finance）。

// stageGroupTask 启动一个待认领的候选组任务实例。
func stageGroupTask(t *testing.T, eng *flow.Engine) (*flow.Instance, string) {
	t.Helper()
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "grp", Data: []byte(groupFlowXML)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "grp", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	tasks, err := eng.ListInstanceTasks(ctx, inst.ID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %d, err = %v", len(tasks), err)
	}
	if tasks[0].Assignee != "" {
		t.Fatalf("group task should be unassigned, got %q", tasks[0].Assignee)
	}
	return inst, tasks[0].ID
}

// ============================================================
// ForwardTask
// ============================================================

func TestForwardTask(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")
	inst, tasks, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "leave_approval", StarterID: "u_emp"})
	if err != nil || len(tasks) != 1 {
		t.Fatalf("start: %v tasks=%d", err, len(tasks))
	}
	taskID := tasks[0]

	if err := eng.ForwardTask(ctx, "ghost", "boss", "carol"); !errors.Is(err, flow.ErrNotFound) {
		t.Errorf("forward ghost task: err = %v, want ErrNotFound", err)
	}
	if err := eng.ForwardTask(ctx, taskID, "someone_else", "carol"); !errors.Is(err, flow.ErrForbidden) {
		t.Errorf("forward by non-assignee: err = %v, want ErrForbidden", err)
	}
	if err := eng.ForwardTask(ctx, taskID, "boss", ""); err == nil {
		t.Error("forward to empty target should fail")
	}
	if err := eng.ForwardTask(ctx, taskID, "boss", "boss"); err == nil {
		t.Error("forward to self should fail")
	}
	if err := eng.ForwardTask(ctx, taskID, "boss", "carol"); err != nil {
		t.Fatalf("forward: %v", err)
	}
	pending, _ := eng.ListPendingTasks(ctx, "t1", "carol")
	if len(pending) != 1 {
		t.Fatalf("carol pending = %d, want 1", len(pending))
	}
	// 转办后可正常办理。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: taskID, ActorID: "carol", Outcome: flow.OutcomeReject,
	}); err != nil {
		t.Fatalf("complete forwarded task: %v", err)
	}
	// 非待办任务不可再转办。
	if err := eng.ForwardTask(ctx, taskID, "carol", "dave"); !errors.Is(err, flow.ErrInvalidState) {
		t.Errorf("forward completed task: err = %v, want ErrInvalidState", err)
	}
	if inst, _ := eng.GetInstance(ctx, inst.ID); inst.Status != flow.InstanceRejected {
		t.Errorf("status = %s, want REJECTED", inst.Status)
	}
}

// ============================================================
// Abort / Withdraw / terminate 边界
// ============================================================

func TestAbortInstance(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")
	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "leave_approval", StarterID: "u_emp"})

	if err := eng.Abort(ctx, "ghost", "admin"); !errors.Is(err, flow.ErrNotFound) {
		t.Errorf("abort ghost: err = %v, want ErrNotFound", err)
	}
	if err := eng.Abort(ctx, inst.ID, "admin"); err != nil {
		t.Fatalf("abort: %v", err)
	}
	got, _ := eng.GetInstance(ctx, inst.ID)
	if got.Status != flow.InstanceAborted {
		t.Fatalf("status = %s, want ABORTED", got.Status)
	}
	// 在途任务应被取消。
	tasks, _ := eng.ListInstanceTasks(ctx, inst.ID)
	for _, tk := range tasks {
		if tk.Status == flow.TaskPending {
			t.Errorf("task %s still pending after abort", tk.ID)
		}
	}
	// 终态实例不可再 Abort。
	if err := eng.Abort(ctx, inst.ID, "admin"); !errors.Is(err, flow.ErrInvalidState) {
		t.Errorf("abort terminal: err = %v, want ErrInvalidState", err)
	}
	// 终态实例 SendMessage 返回 false 而非错误。
	if ok, err := eng.SendMessage(ctx, inst.ID, "anything", nil); err != nil || ok {
		t.Errorf("SendMessage on aborted = (%v, %v), want (false, nil)", ok, err)
	}
}

// TestAbortWithoutStartEvent 绕过部署校验（直接写 store）时引擎应系统终止实例。
func TestAbortWithoutStartEvent(t *testing.T) {
	mem := memory.New()
	ctx := context.Background()
	broken := &flow.Definition{
		Key: "broken", TenantID: "t1", Version: 1, Format: "manual",
		Nodes: map[string]*flow.Node{
			"e": {ID: "e", Type: flow.NodeEndEvent},
		},
	}
	if err := mem.Definitions().Save(ctx, broken); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	log := &recLogger{}
	eng := flow.New(flow.WithStore(mem), flow.WithIdentity(StaticIdentity{}), flow.WithLogger(log))
	if _, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "broken", StarterID: "u0"}); !errors.Is(err, flow.ErrInvalidDefinition) {
		t.Fatalf("start broken: err = %v, want ErrInvalidDefinition", err)
	}
	insts, _ := eng.ListInstancesByStatus(ctx, "t1", flow.InstanceAborted)
	if len(insts) != 1 {
		t.Fatalf("aborted instances = %d, want 1", len(insts))
	}
	if log.count() == 0 {
		t.Error("abort should log reason")
	}
}

// ============================================================
// ClaimTask / UnclaimTask
// ============================================================

func TestClaimUnclaimTask(t *testing.T) {
	eng := newGroupEngine(t)
	ctx := context.Background()
	_, taskID := stageGroupTask(t, eng)

	// 不在候选组 → 拒绝。
	if err := eng.ClaimTask(ctx, taskID, "eve"); !errors.Is(err, flow.ErrForbidden) {
		t.Errorf("claim by outsider: err = %v, want ErrForbidden", err)
	}
	// 候选组用户可认领。
	if err := eng.ClaimTask(ctx, taskID, "fin_a"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := eng.ClaimTask(ctx, taskID, "fin_a"); !errors.Is(err, flow.ErrInvalidState) {
		t.Errorf("double claim: err = %v, want ErrInvalidState", err)
	}
	pending, _ := eng.ListPendingTasks(ctx, "t1", "fin_a")
	if len(pending) != 1 {
		t.Fatalf("fin_a pending = %d, want 1", len(pending))
	}

	// 退认：非本人拒绝，本人放回池子。
	if err := eng.UnclaimTask(ctx, taskID, "fin_b"); !errors.Is(err, flow.ErrForbidden) {
		t.Errorf("unclaim by other: err = %v, want ErrForbidden", err)
	}
	if err := eng.UnclaimTask(ctx, taskID, "fin_a"); err != nil {
		t.Fatalf("unclaim: %v", err)
	}
	cand, _ := eng.ListCandidateTasks(ctx, "t1", "fin_a")
	if len(cand) != 1 {
		t.Fatalf("fin_a candidate tasks = %d, want 1", len(cand))
	}
	// 已退认的任务不是「已认领」，再退认应报错。
	if err := eng.UnclaimTask(ctx, taskID, "fin_a"); !errors.Is(err, flow.ErrInvalidState) {
		t.Errorf("double unclaim: err = %v, want ErrInvalidState", err)
	}
}

func TestClaimTaskRequiresIdentity(t *testing.T) {
	eng := flow.New(flow.WithStore(memory.New()), flow.WithImporters(bpmn.Importer{}))
	ctx := context.Background()
	_, taskID := stageGroupTask(t, eng)
	if err := eng.ClaimTask(ctx, taskID, "fin_a"); !errors.Is(err, flow.ErrNotConfigured) {
		t.Errorf("claim without identity: err = %v, want ErrNotConfigured", err)
	}
}

func TestClaimNonCandidateTask(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")
	_, tasks, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "leave_approval", StarterID: "u_emp"})
	// 直接指派任务已分派 → 不可认领（先撞「已认领」而非「无候选组」）。
	if err := eng.ClaimTask(ctx, tasks[0], "boss"); !errors.Is(err, flow.ErrInvalidState) {
		t.Errorf("claim assigned task: err = %v, want ErrInvalidState", err)
	}
	if err := eng.UnclaimTask(ctx, tasks[0], "boss"); !errors.Is(err, flow.ErrInvalidState) {
		t.Errorf("unclaim assigned task: err = %v, want ErrInvalidState", err)
	}
}

// ============================================================
// 选项：Clock / IDGen / Logger / MaxSteps / Listener
// ============================================================

func TestEngineOptions(t *testing.T) {
	fixed := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	gen := &seqIDGen{}
	log := &recLogger{}
	lis := &recListener{}
	eng := flow.New(
		flow.WithStore(memory.New()),
		flow.WithIdentity(StaticIdentity{}),
		flow.WithImporters(bpmn.Importer{}),
		flow.WithClock(fakeClock{t: fixed}),
		flow.WithIDGen(gen.next),
		flow.WithLogger(log),
		flow.WithListener(lis, flow.ListenerFunc(func(_ context.Context, ev *flow.Event) {
			panic("listener panic must be recovered")
		})),
	)
	ctx := context.Background()
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")

	inst, tasks, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "leave_approval", StarterID: "u_emp",
		Variables: flow.Vars{"days": float64(5)},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !inst.CreatedAt.Equal(fixed) {
		t.Errorf("clock not injected: CreatedAt = %v", inst.CreatedAt)
	}
	if !strings.HasPrefix(inst.ID, "id-") {
		t.Errorf("idgen not injected: inst ID = %q", inst.ID)
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "boss", Outcome: flow.OutcomeApprove, Comment: "同意",
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !lis.has(flow.EventTaskCreated) {
		t.Errorf("listener should receive %s, events = %v", flow.EventTaskCreated, lis.events)
	}
	if log.count() == 0 {
		t.Error("logger should receive deploy log")
	}
}

func TestWithMaxSteps(t *testing.T) {
	eng := flow.New(
		flow.WithStore(memory.New()),
		flow.WithIdentity(StaticIdentity{}),
		flow.WithImporters(bpmn.Importer{}),
		flow.WithMaxSteps(1),
	)
	ctx := context.Background()
	// 3 个节点（start → userTask → end）在 maxSteps=1 下走不完。
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")
	_, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "leave_approval", StarterID: "u_emp"})
	if err == nil || !strings.Contains(err.Error(), "exceeded max steps") {
		t.Fatalf("err = %v, want max steps error", err)
	}
}

// ============================================================
// 定义部署/导出/注册表
// ============================================================

func TestDeployErrors(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	if _, err := eng.Deploy(ctx, flow.DeployRequest{Data: []byte("x")}); err == nil {
		t.Error("deploy without key should fail")
	}
	if _, err := eng.Deploy(ctx, flow.DeployRequest{
		Key: "k", Format: "no-such-format", Data: []byte("x"),
	}); err == nil || !strings.Contains(err.Error(), "no importer registered") {
		t.Errorf("unknown format: err = %v", err)
	}
	// 显式 format 与内容不匹配 → 解析错误。
	if _, err := eng.Deploy(ctx, flow.DeployRequest{
		Key: "k", Format: bpmn.FormatName, Data: []byte(`{"version":2}`),
	}); err == nil {
		t.Error("mismatched format should fail")
	}
	// 校验失败：双起点。
	bad := []byte(`<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
<process id="p"><startEvent id="s1"/><startEvent id="s2"/><endEvent id="e"/>
<sequenceFlow id="f1" sourceRef="s1" targetRef="e"/><sequenceFlow id="f2" sourceRef="s2" targetRef="e"/></process></definitions>`)
	if _, err := eng.Deploy(ctx, flow.DeployRequest{Key: "k", Data: bad}); !errors.Is(err, flow.ErrInvalidDefinition) {
		t.Errorf("invalid definition: err = %v, want ErrInvalidDefinition", err)
	}
}

func TestExportDefinition(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")

	out, err := eng.ExportDefinition(ctx, "t1", "leave_approval", 0, bpmn.FormatName)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(string(out), "<bpmn") && !strings.Contains(string(out), "<definitions") {
		t.Errorf("exported xml unexpected: %.120s", out)
	}
	if _, err := eng.ExportDefinition(ctx, "t1", "leave_approval", 0, "nope"); err == nil {
		t.Error("unknown exporter should fail")
	}
	if _, err := eng.ExportDefinition(ctx, "t1", "ghost", 0, bpmn.FormatName); !errors.Is(err, flow.ErrNotFound) {
		t.Errorf("export ghost: err = %v, want ErrNotFound", err)
	}
}

func TestFormatRegistryDefaultAndDetector(t *testing.T) {
	reg := flow.NewFormatRegistry().RegisterImporter(bpmn.Importer{}).RegisterImporter(windjson.Importer{})

	// 未注册格式：错误信息里应列出已有格式。
	if _, err := reg.ResolveImport("", []byte("x")); err == nil ||
		!strings.Contains(err.Error(), bpmn.FormatName) ||
		!strings.Contains(err.Error(), windjson.FormatName) {
		t.Errorf("no default: err = %v", err)
	}
	// SetDefault 生效。
	reg.SetDefault(bpmn.FormatName)
	im, err := reg.ResolveImport("", []byte("x"))
	if err != nil || im.Format() != bpmn.FormatName {
		t.Errorf("default resolve = %v, %v", im, err)
	}
	// Detector 优先于 default。
	reg.SetDetector(func([]byte) string { return windjson.FormatName })
	im, err = reg.ResolveImport("", []byte("x"))
	if err != nil || im.Format() != windjson.FormatName {
		t.Errorf("detector resolve = %v, %v", im, err)
	}
	// 显式 format 覆盖 detector。
	im, err = reg.ResolveImport(bpmn.FormatName, []byte("x"))
	if err != nil || im.Format() != bpmn.FormatName {
		t.Errorf("explicit resolve = %v, %v", im, err)
	}
}

// ============================================================
// Duration JSON 与 SPI 默认实现
// ============================================================

func TestDurationJSON(t *testing.T) {
	type payload struct {
		Due flow.Duration `json:"due,omitempty"`
	}
	b, err := json.Marshal(payload{Due: flow.Duration(90 * time.Minute)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"due":"1h30m0s"}` {
		t.Errorf("marshal = %s", b)
	}
	var back payload
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Due.Std() != 90*time.Minute {
		t.Errorf("round-trip = %v", back.Due.Std())
	}
	// null / 空串：保持零值。
	var zero payload
	if err := json.Unmarshal([]byte(`{"due":null}`), &zero); err != nil || zero.Due != 0 {
		t.Errorf("null duration = %v, %v", zero.Due, err)
	}
	if err := json.Unmarshal([]byte(`{"due":""}`), &zero); err != nil || zero.Due != 0 {
		t.Errorf("empty duration = %v, %v", zero.Due, err)
	}
	// 非法时长报错。
	if err := json.Unmarshal([]byte(`{"due":"not-a-duration"}`), &zero); err == nil {
		t.Error("bad duration should fail")
	}
}

func TestIsInterruptingDefault(t *testing.T) {
	n := &flow.Node{}
	if !n.IsInterrupting() {
		t.Error("nil Interrupting should default true")
	}
	f := false
	n.Interrupting = &f
	if n.IsInterrupting() {
		t.Error("explicit false should be respected")
	}
}

// ============================================================
// SendMessage 边界
// ============================================================

func TestSendMessageEdges(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")
	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "leave_approval", StarterID: "u_emp"})

	if _, err := eng.SendMessage(ctx, "ghost", "m", nil); !errors.Is(err, flow.ErrNotFound) {
		t.Errorf("send to ghost: err = %v, want ErrNotFound", err)
	}
	// 运行中但无消息等待令牌 → false, nil。
	ok, err := eng.SendMessage(ctx, inst.ID, "no_such_message", flow.Vars{"x": 1})
	if err != nil || ok {
		t.Errorf("send unmatched = (%v, %v), want (false, nil)", ok, err)
	}
}

// TestResumeDueTimersNoDue 无到期令牌时空转返回。
func TestResumeDueTimersNoDue(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	mustDeploy(t, eng, "leave_approval", "examples/leave-approval.bpmn.xml")
	if _, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "leave_approval", StarterID: "u_emp"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	due, err := eng.ResumeDueTimers(ctx, time.Now())
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("due tokens = %d, want 0", len(due))
	}
}

// TestListInstancesByStatusTenantFilter 跨租户隔离。
func TestListInstancesByStatusTenantFilter(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	data, err := os.ReadFile("examples/leave-approval.bpmn.xml")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	// 定义按租户隔离：每个租户各自部署。
	for _, tenant := range []string{"t1", "t2"} {
		if _, err := eng.Deploy(ctx, flow.DeployRequest{
			TenantID: tenant, Key: "leave_approval", Data: data,
		}); err != nil {
			t.Fatalf("deploy %s: %v", tenant, err)
		}
		if _, _, err := eng.Start(ctx, flow.StartRequest{TenantID: tenant, Key: "leave_approval", StarterID: "u_emp"}); err != nil {
			t.Fatalf("start %s: %v", tenant, err)
		}
	}
	got, err := eng.ListInstancesByStatus(ctx, "t1", flow.InstanceRunning)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].TenantID != "t1" {
		t.Fatalf("instances = %+v, want only t1", got)
	}
}
