// 命令 scheduler 是定时调度的参考实现：演示宿主如何用单一 cron 循环
// 驱动 go-wind-flow 的两条时间线——
//
//	ResumeDueTimers  到期定时器节点续跑（引擎自驱）
//	CheckDueTasks    到期用户任务广播 task.due（监听器自决：催办/升级/自动通过）
//
// 调度经 gRPC 服务层（flowserver.Services）走，与生产部署同一条代码路径。
//
// 运行：go run .（在本目录；部署上级目录的 timer-approval.bpmn.xml）
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	"github.com/tx7do/go-wind-flow/store/memory"

	flowv1 "github.com/tx7do/go-wind-flow/api/gen/go/flow/v1"
	flowserver "github.com/tx7do/go-wind-flow/server"
)

func main() {
	ctx := context.Background()
	st := memory.New()
	eng := flow.New(
		flow.WithStore(st),
		flow.WithImporters(bpmn.Importer{}),
		flow.WithExporters(bpmn.Exporter{}),
		flow.WithListener(flow.ListenerFunc(func(_ context.Context, ev *flow.Event) {
			fmt.Printf("[event] %-12s instance=%s node=%s\n", ev.Type, short(ev.InstanceID), ev.NodeID)
		})),
	)

	data, err := os.ReadFile("../timer-approval.bpmn.xml")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read bpmn:", err)
		os.Exit(1)
	}
	if _, err := eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "timer_approval", Data: data}); err != nil {
		fmt.Fprintln(os.Stderr, "deploy:", err)
		os.Exit(1)
	}

	svcs := flowserver.New(eng, st)
	if _, _, err := eng.Start(ctx, flow.StartRequest{
		TenantID: "t1", Key: "timer_approval", StarterID: "u_emp",
	}); err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		os.Exit(1)
	}

	fmt.Println("scheduler started: scanning every 500ms (Ctrl-C to stop)")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()

		// 1) 到期定时器节点：引擎自驱，令牌续走。
		if resumed, err := svcs.ResumeDueTimers(ctx, &flowv1.ResumeDueTimersRequest{
			AsOf: nil, // 缺省 = 当前时刻
		}); err != nil {
			fmt.Fprintln(os.Stderr, "resume timers:", err)
		} else {
			for _, id := range resumed.GetTokenIds() {
				fmt.Printf("[timer ] resumed token %s\n", short(id))
			}
		}

		// 2) 到期用户任务：广播 task.due，催办/升级由监听器决定。
		if due, err := eng.CheckDueTasks(ctx, now); err != nil {
			fmt.Fprintln(os.Stderr, "check due:", err)
		} else {
			for _, t := range due {
				fmt.Printf("[due   ] task %s (assignee=%s) is overdue\n", short(t.ID), t.Assignee)
			}
		}
	}
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
