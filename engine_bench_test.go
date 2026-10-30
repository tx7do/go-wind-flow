package go_wind_flow_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	windjson "github.com/tx7do/go-wind-flow/format/windjson"
	"github.com/tx7do/go-wind-flow/store/memory"
)

// 性能基线（内存存储——衡量引擎自身的状态机开销，不含持久化 IO；
// ent/gorm 的数字取决于具体数据库，宿主另行压测）。
//
// 运行：go test -bench . -benchmem

func benchEngine(b *testing.B) *flow.Engine {
	st := memory.New()
	eng := flow.New(
		flow.WithStore(st),
		flow.WithIdentity(StaticIdentity{}),
		flow.WithImporters(bpmn.Importer{}),
	)
	ctx := context.Background()
	defXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="bench" isExecutable="true">
    <startEvent id="s"/>
    <userTask id="step"><extensionElements><flow:assignees>{"approvers":[{"kind":"STARTER"}]}</flow:assignees></extensionElements></userTask>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="step"/>
    <sequenceFlow id="f2" sourceRef="step" targetRef="e"/>
  </process>
</definitions>`)
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "bench", Data: defXML}); err != nil {
		b.Fatalf("deploy: %v", err)
	}
	return eng
}

// BenchmarkLinearFlow 单任务线性流：Start + Complete = 一次完整审批。
// 万级流转即该数字 × 10000；GOIOP/s 量级参考。
func BenchmarkLinearFlow(b *testing.B) {
	eng := benchEngine(b)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inst, tasks, err := eng.Start(ctx, flow.StartRequest{
			TenantID: "t1", Key: "bench", StarterID: "u1",
		})
		if err != nil {
			b.Fatal(err)
		}
		if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
			TaskID: tasks[0], ActorID: "u1", Outcome: flow.OutcomeApprove,
		}); err != nil {
			b.Fatal(err)
		}
		if inst.Status != flow.InstanceRunning {
			b.Fatal("expected running before completion")
		}
	}
}

// BenchmarkGatewayLoop 网关往返：带条件的排他网关反复选路（表达式求值开销）。
func BenchmarkGatewayLoop(b *testing.B) {
	eng := flow.New(
		flow.WithStore(memory.New()),
		flow.WithImporters(bpmn.Importer{}),
	)
	ctx := context.Background()
	defXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <process id="gw" isExecutable="true">
    <startEvent id="s"/>
    <exclusiveGateway id="g" default="back"/>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="g"/>
    <sequenceFlow id="hit" sourceRef="g" targetRef="e">
      <conditionExpression xsi:type="tFormalExpression">${n &gt;= 0}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="back" sourceRef="g" targetRef="e"/>
  </process>
</definitions>`)
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "gw", Data: defXML}); err != nil {
		b.Fatalf("deploy: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := eng.Start(ctx, flow.StartRequest{
			TenantID: "t1", Key: "gw", StarterID: "u1",
			Variables: flow.Vars{"n": float64(i)},
		}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCounterSign10 十人会签：单任务扇出 10 个待办 + 依次办结。
func BenchmarkCounterSign10(b *testing.B) {
	eng := flow.New(
		flow.WithStore(memory.New()),
		flow.WithImporters(windjson.Importer{}),
	)
	ctx := context.Background()
	defJSON := []byte(fmt.Sprintf(`{
		"version": 2,
		"nodes": [
			{"id": "start", "type": "START"},
			{"id": "sign", "type": "TASK", "approvers": [%s], "strategy": "ALL"},
			{"id": "end", "type": "END"}
		],
		"edges": [{"from": "start", "to": "sign"}, {"from": "sign", "to": "end"}]
	}`, approverJSON(10)))
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "cs10", Data: defJSON}); err != nil {
		b.Fatalf("deploy: %v", err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "cs10", StarterID: "u0"})
		if err != nil {
			b.Fatal(err)
		}
		tasks, _ := eng.ListInstanceTasks(ctx, inst.ID)
		for _, t := range tasks {
			if err := eng.CompleteTask(ctx, flow.CompleteTaskRequest{
				TaskID: t.ID, ActorID: t.Assignee, Outcome: flow.OutcomeApprove,
			}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func approverJSON(n int) string {
	s := ""
	for i := 1; i <= n; i++ {
		if i > 1 {
			s += ","
		}
		s += fmt.Sprintf(`{"type": "USER", "id": %d}`, i)
	}
	return s
}
