package go_wind_flow_test

import (
	"context"
	"testing"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
)

var timerFlowXML = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="timer_flow" isExecutable="true">
    <startEvent id="s"/>
    <intermediateCatchEvent id="wait1">
      <timerEventDefinition>
        <timeDuration>PT2S</timeDuration>
      </timerEventDefinition>
    </intermediateCatchEvent>
    <userTask id="after"><extensionElements><flow:assignees>{"approvers":[{"kind":"USER","value":"later"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="wait1"/>
    <sequenceFlow id="f2" sourceRef="wait1" targetRef="after"/>
    <sequenceFlow id="f3" sourceRef="after" targetRef="e"/>
  </process>
</definitions>`)

func TestIntermediateTimerEvent(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "timer_flow", Data: timerFlowXML}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "timer_flow", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if inst.Status != flow.InstanceRunning {
		t.Fatalf("status = %s, want RUNNING", inst.Status)
	}

	// 未到期：无任务产生、无令牌续跑。
	pending, _ := eng.ListPendingTasks(ctx, "t1", "later")
	if len(pending) != 0 {
		t.Fatalf("task before due should not exist")
	}
	resumed, err := eng.ResumeDueTimers(ctx, time.Now())
	if err != nil || len(resumed) != 0 {
		t.Fatalf("ResumeDueTimers(early) = %d, %v", len(resumed), err)
	}

	// 到期：续跑 → after 节点产生待办。
	resumed, err = eng.ResumeDueTimers(ctx, time.Now().Add(3*time.Second))
	if err != nil || len(resumed) != 1 {
		t.Fatalf("ResumeDueTimers = %d, %v", len(resumed), err)
	}
	pending, _ = eng.ListPendingTasks(ctx, "t1", "later")
	if len(pending) != 1 || pending[0].NodeID != "after" {
		t.Fatalf("pending after resume wrong: %+v", pending)
	}

	// 历史含 timer.fired；重复扫描不再续跑。
	hist, _ := eng.ListHistory(ctx, inst.ID)
	found := false
	for _, h := range hist {
		if h.Type == flow.EventTimerFired {
			found = true
		}
	}
	if !found {
		t.Fatal("timer.fired event missing")
	}
	resumed, _ = eng.ResumeDueTimers(ctx, time.Now().Add(10*time.Second))
	if len(resumed) != 0 {
		t.Fatalf("second resume should pick nothing, got %d", len(resumed))
	}

	// 完成后续走正常终结。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: pending[0].ID, ActorID: "later", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}
}
