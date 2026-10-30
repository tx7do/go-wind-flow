package go_wind_flow_test

import (
	"context"
	"testing"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
)

// 中断型：start → review(STARTER, 挂 1s 边界定时器) ─┬→ escalate(later) → end
//
//	└(正常通过)→ ok → end2
const interruptingXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="boundary_int" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="review" name="审批"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"STARTER"}]}</flow:assignees></extensionElements></userTask>
    <boundaryEvent id="timeout" attachedToRef="review" cancelActivity="true">
      <timerEventDefinition><timeDuration>PT1S</timeDuration></timerEventDefinition>
    </boundaryEvent>
    <userTask id="escalate" name="升级处理"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"later"}]}</flow:assignees></extensionElements></userTask>
    <userTask id="ok" name="通过后归档"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"archiver"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e1"/>
    <endEvent id="e2"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <sequenceFlow id="fb" sourceRef="timeout" targetRef="escalate"/>
    <sequenceFlow id="f2" sourceRef="escalate" targetRef="e1"/>
    <sequenceFlow id="f3" sourceRef="review" targetRef="ok"/>
    <sequenceFlow id="f4" sourceRef="ok" targetRef="e2"/>
  </process>
</definitions>`

// 非中断型：到期只派生升级分支，原审批照常可办。
const nonInterruptingXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="boundary_non" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="review" name="审批"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"STARTER"}]}</flow:assignees></extensionElements></userTask>
    <boundaryEvent id="remind" attachedToRef="review" cancelActivity="false">
      <timerEventDefinition><timeDuration>PT1S</timeDuration></timerEventDefinition>
    </boundaryEvent>
    <userTask id="escalate" name="升级处理"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"later"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e1"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <sequenceFlow id="fb" sourceRef="remind" targetRef="escalate"/>
    <sequenceFlow id="f2" sourceRef="escalate" targetRef="e1"/>
    <sequenceFlow id="f3" sourceRef="review" targetRef="e1"/>
  </process>
</definitions>`

func deployBoundary(t *testing.T, key string, data []byte) *flow.Engine {
	t.Helper()
	eng := newEngine(t)
	if _, err := eng.Deploy(context.Background(), flow.DeployRequest{
		TenantID: "t1", Key: key, Data: data,
	}); err != nil {
		t.Fatalf("deploy %s: %v", key, err)
	}
	return eng
}

func TestBoundaryInterrupting(t *testing.T) {
	eng := deployBoundary(t, "b_int", []byte(interruptingXML))
	ctx := context.Background()

	inst, tasks, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "b_int", StarterID: "u_emp"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}

	// 未到期：无升级任务。
	resumed, err := eng.ResumeDueTimers(ctx, time.Now())
	if err != nil || len(resumed) != 0 {
		t.Fatalf("early resume = %d, %v", len(resumed), err)
	}

	// 到期：原任务取消，升级任务出现。
	if _, err := eng.ResumeDueTimers(ctx, time.Now().Add(2*time.Second)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	all, _ := eng.ListInstanceTasks(ctx, inst.ID)
	for _, tk := range all {
		if tk.NodeID == "review" && tk.Status == flow.TaskPending {
			t.Fatal("review task should be CANCELLED after interrupting boundary fired")
		}
	}
	esc, _ := eng.ListPendingTasks(ctx, "t1", "later")
	if len(esc) != 1 || esc[0].NodeID != "escalate" {
		t.Fatalf("escalate task wrong: %+v", esc)
	}

	// 升级办结 → APPROVED。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: esc[0].ID, ActorID: "later", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete escalate: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}
}

func TestBoundaryCompleteBeforeDue(t *testing.T) {
	eng := deployBoundary(t, "b_early", []byte(interruptingXML))
	ctx := context.Background()

	inst, tasks, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "b_early", StarterID: "u_emp"})

	// 提前办结 → 边界定时器清理，永远不触发。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "u_emp", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete review: %v", err)
	}
	if _, err := eng.ResumeDueTimers(ctx, time.Now().Add(2*time.Second)); err != nil {
		t.Fatalf("resume after completion: %v", err)
	}
	esc, _ := eng.ListPendingTasks(ctx, "t1", "later")
	if len(esc) != 0 {
		t.Fatalf("escalate should never fire, got %d", len(esc))
	}
	// 走归档分支收尾。
	arch, _ := eng.ListPendingTasks(ctx, "t1", "archiver")
	if len(arch) != 1 {
		t.Fatalf("archive task missing")
	}
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: arch[0].ID, ActorID: "archiver", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete archive: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s", inst.Status)
	}
}

func TestBoundaryNonInterrupting(t *testing.T) {
	eng := deployBoundary(t, "b_non", []byte(nonInterruptingXML))
	ctx := context.Background()

	inst, _, _ := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "b_non", StarterID: "u_emp"})

	// 到期：升级分支派生，原任务仍待办。
	if _, err := eng.ResumeDueTimers(ctx, time.Now().Add(2*time.Second)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	esc, _ := eng.ListPendingTasks(ctx, "t1", "later")
	if len(esc) != 1 || esc[0].NodeID != "escalate" {
		t.Fatalf("escalate task wrong: %+v", esc)
	}
	review, _ := eng.ListPendingTasks(ctx, "t1", "u_emp")
	if len(review) != 1 || review[0].NodeID != "review" {
		t.Fatal("non-interrupting should keep review pending")
	}

	// 原审批 + 升级都办结才 APPROVED（两支各一令牌）。
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: review[0].ID, ActorID: "u_emp", Outcome: flow.OutcomeApprove,
	})
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceRunning {
		t.Fatalf("status = %s, want RUNNING (escalate branch live)", inst.Status)
	}
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: esc[0].ID, ActorID: "later", Outcome: flow.OutcomeApprove,
	})
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}
}
