package go_wind_flow_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
)

func deployTimerAction(t *testing.T, action string) (*flow.Engine, *flow.Instance, string) {
	t.Helper()
	eng := newEngine(t)
	// start → quick（due=1ns，到期动作）→ next（STARTER）→ end
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="ta" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="quick" name="限时审批"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"STARTER"}],"due":"1ns","action":"` + action + `"}</flow:assignees></extensionElements></userTask>
    <userTask id="next" name="后续"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"next_user"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="quick"/>
    <sequenceFlow id="f2" sourceRef="quick" targetRef="next"/>
    <sequenceFlow id="f3" sourceRef="next" targetRef="e"/>
  </process>
</definitions>`
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "ta", Data: []byte(xml)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	inst, tasks, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "ta", StarterID: "u_emp"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return eng, inst, tasks[0]
}

func TestTimerActionAutoApprove(t *testing.T) {
	eng, inst, taskID := deployTimerAction(t, "auto_approve")
	ctx := context.Background()

	// 到期扫描：引擎自动通过（系统操作者署名），令牌推进到 next。
	if _, err := eng.CheckDueTasks(ctx, time.Now().Add(time.Second)); err != nil {
		t.Fatalf("check due: %v", err)
	}
	all, _ := eng.ListInstanceTasks(ctx, inst.ID)
	for _, tk := range all {
		if tk.NodeID == "quick" {
			if tk.Status != flow.TaskApproved || tk.Outcome != flow.OutcomeApprove {
				t.Fatalf("quick should be auto-approved, got %s/%s", tk.Status, tk.Outcome)
			}
		}
	}
	next, _ := eng.ListPendingTasks(ctx, "t1", "next_user")
	if len(next) != 1 {
		t.Fatalf("flow should have advanced, next pending = %d", len(next))
	}

	// 自动通过的任务不能再被人工办结（已非 PENDING）。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: taskID, ActorID: "u_emp", Outcome: flow.OutcomeApprove,
	}); err == nil {
		t.Fatal("completing auto-approved task should fail")
	}
	_ = ctx
}

func TestTimerActionAutoReject(t *testing.T) {
	eng, inst, taskID := deployTimerAction(t, "auto_reject")
	ctx := context.Background()

	if _, err := eng.CheckDueTasks(ctx, time.Now().Add(time.Second)); err != nil {
		t.Fatalf("check due: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceRejected {
		t.Fatalf("status = %s, want REJECTED (auto reject, TERMINATE mode)", inst.Status)
	}
	all, _ := eng.ListInstanceTasks(ctx, inst.ID)
	for _, tk := range all {
		if tk.ID == taskID && (tk.Status != flow.TaskRejected || tk.Outcome != flow.OutcomeReject) {
			t.Fatalf("task should be auto-rejected, got %s/%s", tk.Status, tk.Outcome)
		}
	}
}

func TestTimerActionDefaultBroadcastOnly(t *testing.T) {
	eng, inst, taskID := deployTimerAction(t, "")
	ctx := context.Background()

	// 缺省动作：只广播 task.due，任务保持待办。
	if _, err := eng.CheckDueTasks(ctx, time.Now().Add(time.Second)); err != nil {
		t.Fatalf("check due: %v", err)
	}
	all, _ := eng.ListInstanceTasks(ctx, inst.ID)
	for _, tk := range all {
		if tk.ID == taskID && tk.Status != flow.TaskPending {
			t.Fatalf("default action should keep task pending, got %s", tk.Status)
		}
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceRunning {
		t.Fatalf("status = %s, want RUNNING", inst.Status)
	}
	_ = strings.TrimSpace("")
}

// Camunda Modeler 风格导出回环：完整命名空间、documentation、
// incoming/outgoing 子元素、camunda: 扩展属性、bpmndi/DI —— 解析不报错且
// 图形回填无损。
func TestCamundaStyleRoundTrip(t *testing.T) {
	camundaXML := `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
                  xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
                  xmlns:dc="http://www.omg.org/spec/DD/20100524/DC"
                  xmlns:di="http://www.omg.org/spec/DD/20100524/DI"
                  xmlns:camunda="http://camunda.org/schema/1.0/bpmn"
                  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
                  id="Definitions_1" targetNamespace="http://bpmn.io/schema/bpmn">
  <bpmn:process id="expense" name="报销审批" isExecutable="true">
    <bpmn:startEvent id="StartEvent_1" name="提交">
      <bpmn:documentation>报销申请入口</bpmn:documentation>
      <bpmn:outgoing>Flow_1</bpmn:outgoing>
    </bpmn:startEvent>
    <bpmn:userTask id="Activity_review" name="领导审批" camunda:assignee="leader">
      <bpmn:documentation>金额超过 1000 元需领导审批</bpmn:documentation>
      <bpmn:extensionElements>
        <flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"LEADER"}]}</flow:assignees>
      </bpmn:extensionElements>
      <bpmn:incoming>Flow_1</bpmn:incoming>
      <bpmn:outgoing>Flow_2</bpmn:outgoing>
    </bpmn:userTask>
    <bpmn:endEvent id="EndEvent_1" name="结束">
      <bpmn:incoming>Flow_2</bpmn:incoming>
    </bpmn:endEvent>
    <bpmn:sequenceFlow id="Flow_1" sourceRef="StartEvent_1" targetRef="Activity_review"/>
    <bpmn:sequenceFlow id="Flow_2" sourceRef="Activity_review" targetRef="EndEvent_1"/>
  </bpmn:process>
  <bpmndi:BPMNDiagram id="BPMNDiagram_1">
    <bpmndi:BPMNPlane id="BPMNPlane_1" bpmnElement="expense">
      <bpmndi:BPMNShape id="StartEvent_1_di" bpmnElement="StartEvent_1">
        <dc:Bounds x="152" y="102" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="Activity_review_di" bpmnElement="Activity_review">
        <dc:Bounds x="240" y="80" width="100" height="80"/>
      </bpmndi:BPMNShape>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
</bpmn:definitions>`

	eng := newEngine(t)
	ctx := context.Background()
	def, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "camunda", Data: []byte(camundaXML)})
	if err != nil {
		t.Fatalf("deploy camunda xml: %v", err)
	}
	if def.Nodes["Activity_review"].Documentation != "金额超过 1000 元需领导审批" {
		t.Fatalf("documentation lost: %q", def.Nodes["Activity_review"].Documentation)
	}
	if def.Nodes["StartEvent_1"].Documentation != "报销申请入口" {
		t.Fatalf("start documentation lost")
	}

	out, err := eng.ExportDefinition(ctx, "t1", "camunda", 0, "bpmn-xml")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		// 原命名空间声明与 DI 图形保留；根元素前缀与闭合匹配；默认命名空间注入
		`xmlns:bpmndi=`, `<dc:Bounds x="240"`, `<bpmn:definitions `, `</bpmn:definitions>`,
		`xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"`,
		"<documentation>金额超过 1000 元需领导审批</documentation>",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("export missing %q", want)
		}
	}

	// 导出结果可再次部署（回环）。
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "camunda_v2", Data: out}); err != nil {
		t.Fatalf("re-deploy: %v", err)
	}
}
