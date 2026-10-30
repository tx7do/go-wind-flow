package go_wind_flow_test

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
)

// start → 等待外部支付回调（消息） → 审批 → 结束。
const messageFlowXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="msg_flow" isExecutable="true">
    <startEvent id="s"/>
    <intermediateCatchEvent id="wait_pay">
      <messageEventDefinition messageRef="payment_received"/>
    </intermediateCatchEvent>
    <userTask id="review"><extensionElements><flow:assignees xmlns:flow="https://github.com/tx7do/go-wind-flow">{"approvers":[{"kind":"USER","value":"fin"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="wait_pay"/>
    <sequenceFlow id="f2" sourceRef="wait_pay" targetRef="review"/>
    <sequenceFlow id="f3" sourceRef="review" targetRef="e"/>
  </process>
</definitions>`

func TestMessageEvent(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "msg_flow", Data: []byte(messageFlowXML)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "msg_flow", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// 阻塞中：无待办。
	pending, _ := eng.ListPendingTasks(ctx, "t1", "fin")
	if len(pending) != 0 {
		t.Fatal("no task should exist while waiting for message")
	}

	// 错误消息名：不投递。
	delivered, err := eng.SendMessage(ctx, inst.ID, "wrong_message", nil)
	if err != nil || delivered {
		t.Fatalf("wrong message should not deliver, got %v/%v", delivered, err)
	}

	// 正确投递：payload 并入实例变量，令牌续走产生审批任务。
	delivered, err = eng.SendMessage(ctx, inst.ID, "payment_received", flow.Vars{"paid": true, "amount": float64(99)})
	if err != nil || !delivered {
		t.Fatalf("SendMessage = %v, %v", delivered, err)
	}
	pending, _ = eng.ListPendingTasks(ctx, "t1", "fin")
	if len(pending) != 1 {
		t.Fatalf("pending after message = %d, want 1", len(pending))
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if v, ok := inst.Variables["paid"]; !ok || v != true {
		t.Fatalf("payload not merged: %+v", inst.Variables)
	}

	// 重复投递：令牌已走，不再命中。
	delivered, _ = eng.SendMessage(ctx, inst.ID, "payment_received", nil)
	if delivered {
		t.Fatal("second delivery should miss")
	}

	// 办结 → APPROVED；历史含 message.received。
	if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: pending[0].ID, ActorID: "fin", Outcome: flow.OutcomeApprove,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s", inst.Status)
	}
	hist, _ := eng.ListHistory(ctx, inst.ID)
	found := false
	for _, h := range hist {
		if h.Type == flow.EventMessageReceived {
			found = true
		}
	}
	if !found {
		t.Fatal("message.received event missing")
	}
}
