package go_wind_flow_test

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	"github.com/tx7do/go-wind-flow/store/memory"
)

// collectionEval 求值器替身：变量 items → 数组。
type collectionEvaluator struct {
	flow.SimpleEvaluator
}

func (collectionEvaluator) EvalAny(_ context.Context, expression string, env flow.Vars) (any, error) {
	if expression == "items" {
		return []any{"a1", "a2"}, nil
	}
	return flow.SimpleEvaluator{}.EvalAny(context.Background(), expression, env)
}

// 集合多实例（串行 + elementVar）。
var collectionFlowXML = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="col_mi" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="step" name="逐项审批">
      <extensionElements>
        <flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"collection":"items","elementVar":"item"}</flow:assignees>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="true"/>
    </userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="step"/>
    <sequenceFlow id="f2" sourceRef="step" targetRef="e"/>
  </process>
</definitions>`)

func TestCollectionMultiInstanceSequential(t *testing.T) {
	eng := flow.New(
		flow.WithStore(memory.New()),
		flow.WithIdentity(StaticIdentity{}),
		flow.WithEvaluator(collectionEvaluator{}),
		flow.WithImporters(bpmn.Importer{}),
	)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{
		TenantID: "t1", Key: "col_mi", Data: collectionFlowXML,
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	inst, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "col_mi", StarterID: "u0",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// 串行逐项：a1 → a2，每时刻一个待办。
	for _, uid := range []string{"a1", "a2"} {
		tasks, _ := eng.ListPendingTasks(ctx, "t1", uid)
		if len(tasks) != 1 {
			t.Fatalf("pending for %s = %d, want 1", uid, len(tasks))
		}
		all, _ := eng.ListInstanceTasks(ctx, inst.ID)
		pending := 0
		for _, x := range all {
			if x.Status == flow.TaskPending {
				pending++
			}
		}
		if pending != 1 {
			t.Fatalf("pending total after %s = %d, want 1 (serial)", uid, pending)
		}
		if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
			TaskID: tasks[0].ID, ActorID: uid, Outcome: flow.OutcomeApprove,
		}); err != nil {
			t.Fatalf("complete %s: %v", uid, err)
		}
	}

	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}
}

// 嵌入子流程（SES）扁平化：外部 start → sub(start → inner task → end) → 归档 → end。
var embedFlowXML = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="embed" isExecutable="true">
    <startEvent id="s"/>
    <subProcess id="sub" name="预处理">
      <startEvent id="sub_s"/>
      <userTask id="prep"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"STARTER"}]}</flow:assignees></extensionElements></userTask>
      <endEvent id="sub_e"/>
      <sequenceFlow id="sf1" sourceRef="sub_s" targetRef="prep"/>
      <sequenceFlow id="sf2" sourceRef="prep" targetRef="sub_e"/>
    </subProcess>
    <userTask id="final"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"u9"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="sub"/>
    <sequenceFlow id="f2" sourceRef="sub" targetRef="final"/>
    <sequenceFlow id="f3" sourceRef="final" targetRef="e"/>
  </process>
</definitions>`)

func TestEmbedSubProcessFlatten(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	def, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "embed", Data: embedFlowXML})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}

	// 扁平化：sub 自身不出现，内部节点带前缀并入。
	if _, ok := def.Nodes["sub"]; ok {
		t.Fatal("sub node should be flattened away")
	}
	for _, id := range []string{"sub.prep"} {
		if _, ok := def.Nodes[id]; !ok {
			t.Fatalf("flattened node %s missing", id)
		}
	}
	if err := flow.ValidateDefinition(def); err != nil {
		t.Fatalf("validate flattened: %v", err)
	}

	// 端到端：内部 STARTER 任务 → 归档任务，流程跨扁平边界正常流转。
	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "embed", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	inner, _ := eng.ListPendingTasks(ctx, "t1", "u0")
	if len(inner) != 1 {
		t.Fatalf("inner task missing: %d", len(inner))
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: inner[0].ID, ActorID: "u0", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete inner: %v", err)
	}
	final, _ := eng.ListPendingTasks(ctx, "t1", "u9")
	if len(final) != 1 {
		t.Fatalf("flow did not exit subprocess: %d", len(final))
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: final[0].ID, ActorID: "u9", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete final: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s", inst.Status)
	}
}
