package go_wind_flow_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

// 顺序多实例（isSequential=true）：候选 11 → 22 → 33 串行逐个审批。
const seqMiXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="seq_mi" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="chain" name="逐级审批">
      <extensionElements>
        <flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"u_11"},{"kind":"USER","value":"u_22"},{"kind":"USER","value":"u_33"}]}</flow:assignees>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="true">
        <completionCondition>ALL</completionCondition>
      </multiInstanceLoopCharacteristics>
    </userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="chain"/>
    <sequenceFlow id="f2" sourceRef="chain" targetRef="e"/>
  </process>
</definitions>`

func seqPending(t *testing.T, eng *flow.Engine, assignee string) *flow.Task {
	t.Helper()
	tasks, err := eng.ListPendingTasks(context.Background(), "t1", assignee)
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("no pending task for %s", assignee)
	}
	return tasks[0]
}

func TestSequentialMultiInstance(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "seq_mi", Data: []byte(seqMiXML)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "seq_mi", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// 串行：同一时刻只有 u_11 的任务，且 u_22/u_33 尚无任务。
	t1 := seqPending(t, eng, "u_11")
	if t1.NodeID != "chain" {
		t.Fatalf("node = %s", t1.NodeID)
	}
	if p, _ := eng.ListPendingTasks(ctx, "t1", "u_22"); len(p) != 0 {
		t.Fatal("u_22 should not have a task yet (sequential)")
	}
	_ = t1

	// 依次推进：11 → 22 → 33。
	for _, uid := range []string{"u_11", "u_22"} {
		tk := seqPending(t, eng, uid)
		if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
			TaskID: tk.ID, ActorID: uid, Outcome: flow.OutcomeApprove,
		}); err != nil {
			t.Fatalf("complete %s: %v", uid, err)
		}
		// 每一步只有一个待办（串行不并发派生）。
		all, _ := eng.ListInstanceTasks(ctx, inst.ID)
		pending := 0
		for _, x := range all {
			if x.Status == flow.TaskPending {
				pending++
			}
		}
		if pending != 1 {
			t.Fatalf("pending after %s = %d, want 1", uid, pending)
		}
	}

	// 中途驳回 → 实例终结（fail-closed 默认语义）。
	tk := seqPending(t, eng, "u_33")
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tk.ID, ActorID: "u_33", Outcome: flow.OutcomeReject,
	}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceRejected {
		t.Fatalf("status = %s, want REJECTED", inst.Status)
	}
}

func TestSequentialMultiInstanceApproveAll(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "seq_mi", Data: []byte(seqMiXML)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "seq_mi", StarterID: "u0"})
	for _, uid := range []string{"u_11", "u_22", "u_33"} {
		tk := seqPending(t, eng, uid)
		if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
			TaskID: tk.ID, ActorID: uid, Outcome: flow.OutcomeApprove,
		}); err != nil {
			t.Fatalf("complete %s: %v", uid, err)
		}
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}
}

func TestDocumentationAndDIPassthrough(t *testing.T) {
	src := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
             xmlns:dc="http://www.omg.org/spec/DD/20100524/DC">
  <process id="doc_di" name="文档与图形" isExecutable="true">
    <documentation>全流程说明文本</documentation>
    <startEvent id="s"/>
    <userTask id="review" name="审批">
      <documentation>请核对报销明细</documentation>
      <extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"u1"}]}</flow:assignees></extensionElements>
    </userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <sequenceFlow id="f2" sourceRef="review" targetRef="e"/>
  </process>
  <bpmndi:BPMNDiagram id="BPMNDiagram_1">
    <bpmndi:BPMNPlane id="BPMNPlane_1" bpmnElement="doc_di">
      <bpmndi:BPMNShape id="s_di" bpmnElement="s">
        <dc:Bounds x="100" y="200" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="review_di" bpmnElement="review">
        <dc:Bounds x="200" y="178" width="100" height="80"/>
      </bpmndi:BPMNShape>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
</definitions>`)

	eng := newEngine(t)
	ctx := context.Background()
	def, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "doc_di", Data: src})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}

	// documentation 透传到领域模型（process 级文档不落 Node，仅节点级透传）。
	if def.Nodes["review"].Documentation != "请核对报销明细" {
		t.Fatalf("documentation lost: %q", def.Nodes["review"].Documentation)
	}

	// 导出：DI 图形回填 + 命名空间保留。
	out, err := eng.ExportDefinition(ctx, "t1", "doc_di", 0, "bpmn-xml")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"bpmndi:BPMNDiagram", `x="200"`, "dc:Bounds",
		"<documentation>请核对报销明细</documentation>", "xmlns:bpmndi=",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("export missing %q", want)
		}
	}

	// 回环：导出结果可再次部署且节点不丢。
	def2, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "doc_di_v2", Data: out})
	if err != nil {
		t.Fatalf("re-deploy exported: %v", err)
	}
	if len(def2.Nodes) != len(def.Nodes) {
		t.Fatalf("re-deploy node count = %d, want %d", len(def2.Nodes), len(def.Nodes))
	}
	if def2.Nodes["review"].Documentation != "请核对报销明细" {
		t.Fatal("documentation lost on re-deploy")
	}
}
