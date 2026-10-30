package entstore

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
)

// 真实存储（ent + sqlite）性能基线：衡量 WithTx 事务 + 五表落库的整体开销，
// 与根目录内存基线（engine_bench_test.go）对照阅读。
func BenchmarkEntLinearFlow(b *testing.B) {
	db := newBenchDB(b)
	st := New(db)
	eng := flow.New(flow.WithStore(st), flow.WithImporters(bpmn.Importer{}))
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
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := st.WithTx(ctx, func(ctx context.Context) error {
			_, tasks, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "bench", StarterID: "u1"})
			if err != nil {
				return err
			}
			return eng.CompleteTask(ctx, flow.CompleteTaskRequest{
				TaskID: tasks[0], ActorID: "u1", Outcome: flow.OutcomeApprove,
			})
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
