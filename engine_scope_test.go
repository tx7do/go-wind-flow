package go_wind_flow_test

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	"github.com/tx7do/go-wind-flow/store/memory"
)

// scriptStub 支持 scope 语义：code 形如 "local:on" / "global:on"。
type scopedScriptStub struct{}

func (scopedScriptStub) Run(_ context.Context, spec *flow.ScriptSpec, env flow.Vars) (any, error) {
	code := spec.Code
	switch {
	case code == "local:on":
		if spec.Scope == "local" {
			return flow.Vars{"local_on": true}, nil
		}
		return nil, nil
	case code == "global:on":
		return flow.Vars{"global_on": true}, nil
	}
	return nil, nil
}

var scopeFlowXML = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
             xmlns:flow="https://github.com/tx7do/go-wind-flow">
  <process id="scope" isExecutable="true">
    <startEvent id="s"/>
    <scriptTask id="w_local" scriptFormat="js" flow:scope="local"><script>local:on</script></scriptTask>
    <exclusiveGateway id="g1" default="g1d"/>
    <scriptTask id="w_global" scriptFormat="js" flow:scope="instance"><script>global:on</script></scriptTask>
    <exclusiveGateway id="g2" default="g2d"/>
    <endEvent id="e"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="w_local"/>
    <sequenceFlow id="f2" sourceRef="w_local" targetRef="g1"/>
    <sequenceFlow id="hit1" sourceRef="g1" targetRef="w_global">
      <conditionExpression xsi:type="tFormalExpression">${local_on == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="g1d" sourceRef="g1" targetRef="e"/>
    <sequenceFlow id="f3" sourceRef="w_global" targetRef="g2"/>
    <sequenceFlow id="hit2" sourceRef="g2" targetRef="e">
      <conditionExpression xsi:type="tFormalExpression">${local_on == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="g2d" sourceRef="g2" targetRef="e"/>
  </process>
</definitions>`)

func TestVariableScoping(t *testing.T) {
	eng := flow.New(
		flow.WithStore(memory.New()),
		flow.WithScripts(scopedScriptStub{}),
		flow.WithImporters(bpmn.Importer{}),
	)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "scope", Data: scopeFlowXML}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "scope", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// 链路语义：w_local 写局部 local_on → g1 命中（读侧叠加）→ w_global 写实例
	// global_on → g2 的 local_on 条件不成立（局部不跨脚本任务残留）→ 走默认终结。
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED", inst.Status)
	}

	// 局部变量不污染实例：实例里只有 global_on。
	if _, ok := inst.Variables["local_on"]; ok {
		t.Fatalf("local_on leaked into instance vars: %+v", inst.Variables)
	}
	if v, ok := inst.Variables["global_on"]; !ok || v != true {
		t.Fatalf("global_on missing: %+v", inst.Variables)
	}

	// 历史里只有实例级 variable.changed（局部写不发该事件）。
	hist, _ := eng.ListHistory(ctx, inst.ID)
	changed := 0
	for _, h := range hist {
		if h.Type == flow.EventVariableChanged {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("variable.changed events = %d, want 1 (global only)", changed)
	}
}

var forkScopeXML = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
             xmlns:flow="https://github.com/tx7do/go-wind-flow">
  <process id="fork_scope" isExecutable="true">
    <startEvent id="s"/>
    <parallelGateway id="fork"/>
    <scriptTask id="la" scriptFormat="js" flow:scope="local"><script>local:on</script></scriptTask>
    <scriptTask id="lb" scriptFormat="js" flow:scope="local"><script>local:on</script></scriptTask>
    <exclusiveGateway id="ga" default="gad"/>
    <exclusiveGateway id="gb" default="gbd"/>
    <endEvent id="enda"/>
    <endEvent id="endb"/>
    <parallelGateway id="join"/>
    <endEvent id="e"/>
    <sequenceFlow id="f0" sourceRef="s" targetRef="fork"/>
    <sequenceFlow id="fa" sourceRef="fork" targetRef="la"/>
    <sequenceFlow id="fb" sourceRef="fork" targetRef="lb"/>
    <sequenceFlow id="fa2" sourceRef="la" targetRef="ga"/>
    <sequenceFlow id="fb2" sourceRef="lb" targetRef="gb"/>
    <sequenceFlow id="hita" sourceRef="ga" targetRef="join">
      <conditionExpression xsi:type="tFormalExpression">${local_on == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="gad" sourceRef="ga" targetRef="enda"/>
    <sequenceFlow id="hitb" sourceRef="gb" targetRef="join">
      <conditionExpression xsi:type="tFormalExpression">${local_on == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="gbd" sourceRef="gb" targetRef="endb"/>
    <sequenceFlow id="fj" sourceRef="join" targetRef="e"/>
  </process>
</definitions>`)

func TestForkLocalsIsolation(t *testing.T) {
	eng := flow.New(
		flow.WithStore(memory.New()),
		flow.WithScripts(scopedScriptStub{}),
		flow.WithImporters(bpmn.Importer{}),
	)
	ctx := context.Background()
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "fork_scope", Data: forkScopeXML}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	inst, _, err := eng.Start(ctx, flow.StartRequest{TenantID: "t1", Key: "fork_scope", StarterID: "u0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	inst, _ = eng.GetInstance(ctx, inst.ID)
	if inst.Status != flow.InstanceApproved {
		t.Fatalf("status = %s, want APPROVED (两支局部各自命中)", inst.Status)
	}
	if _, ok := inst.Variables["local_on"]; ok {
		t.Fatalf("local_on leaked into instance vars")
	}
}
