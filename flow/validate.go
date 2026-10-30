package flow

import "fmt"

// ValidateDefinition 部署前校验。规则对齐 go-wind-oa/im 引擎的既有约束并泛化：
//
//   - 恰一个 startEvent；至少一个 endEvent；
//   - start：0 入 1 出；end：≥1 入 0 出；
//   - userTask/scriptTask/serviceTask/callActivity：1 入 1 出；
//   - 排他网关：≥2 出，非默认出边必须带条件，至多一条默认流，默认流必须存在；
//   - 并行/包容网关：分叉 ≥2 出（出边无条件）或汇合 ≥2 入（1 出），二选一或兼有；
//   - 脚本任务必须带 ScriptSpec；callActivity 必须指定 SubprocessKey；
//   - 全节点自 start 可达，且均可达某 end（活性保证）。
func ValidateDefinition(def *Definition) error {
	if len(def.Nodes) == 0 {
		return fmt.Errorf("%w: no nodes", ErrInvalidDefinition)
	}

	startCount, endCount := 0, 0
	for id, n := range def.Nodes {
		if id == "" || n.Type == "" {
			return fmt.Errorf("%w: node missing id or type", ErrInvalidDefinition)
		}
		switch n.Type {
		case NodeStartEvent:
			startCount++
		case NodeEndEvent:
			endCount++
		case NodeUserTask, NodeScriptTask, NodeServiceTask, NodeCallActivity, NodeTimerEvent, NodeBoundaryEvent:
		case NodeExclusiveGateway, NodeParallelGateway, NodeInclusiveGateway:
		default:
			return fmt.Errorf("%w: unknown node type %q (%s)", ErrInvalidDefinition, n.Type, id)
		}
	}
	if startCount != 1 {
		return fmt.Errorf("%w: want exactly 1 startEvent, got %d", ErrInvalidDefinition, startCount)
	}
	if endCount < 1 {
		return fmt.Errorf("%w: want at least 1 endEvent, got %d", ErrInvalidDefinition, endCount)
	}

	for _, e := range def.Edges {
		if def.Nodes[e.From] == nil || def.Nodes[e.To] == nil {
			return fmt.Errorf("%w: edge %s references unknown node (%s -> %s)",
				ErrInvalidDefinition, e.ID, e.From, e.To)
		}
	}

	for id, n := range def.Nodes {
		out := def.OutEdges(id)
		in := def.InDegree(id)
		switch n.Type {
		case NodeStartEvent:
			if in != 0 || len(out) != 1 {
				return fmt.Errorf("%w: startEvent %s must have 0 in / 1 out", ErrInvalidDefinition, id)
			}
		case NodeEndEvent:
			if in < 1 || len(out) != 0 {
				return fmt.Errorf("%w: endEvent %s must have ≥1 in / 0 out", ErrInvalidDefinition, id)
			}
		case NodeUserTask, NodeScriptTask, NodeServiceTask, NodeCallActivity:
			if in != 1 || len(out) != 1 {
				return fmt.Errorf("%w: node %s (%s) must have 1 in / 1 out", ErrInvalidDefinition, id, n.Type)
			}
			if (n.Type == NodeScriptTask || n.Type == NodeServiceTask) && n.Script == nil {
				return fmt.Errorf("%w: script task %s missing script spec", ErrInvalidDefinition, id)
			}
			if n.Type == NodeCallActivity && n.SubprocessKey == "" {
				return fmt.Errorf("%w: callActivity %s missing subprocess key", ErrInvalidDefinition, id)
			}
		case NodeTimerEvent:
			if in != 1 || len(out) != 1 {
				return fmt.Errorf("%w: timer event %s must have 1 in / 1 out", ErrInvalidDefinition, id)
			}
			if n.Timer == nil || n.Timer.Duration.Std() <= 0 {
				return fmt.Errorf("%w: timer event %s missing positive duration", ErrInvalidDefinition, id)
			}
		case NodeBoundaryEvent:
			if in != 0 || len(out) < 1 {
				return fmt.Errorf("%w: boundary event %s must have 0 in / ≥1 out", ErrInvalidDefinition, id)
			}
			if n.AttachedTo == "" {
				return fmt.Errorf("%w: boundary event %s missing attachedToRef", ErrInvalidDefinition, id)
			}
			attached := def.Nodes[n.AttachedTo]
			if attached == nil {
				return fmt.Errorf("%w: boundary event %s attached to unknown node %s",
					ErrInvalidDefinition, id, n.AttachedTo)
			}
			if attached.Type != NodeUserTask {
				return fmt.Errorf("%w: boundary event %s can only attach to userTask, got %s",
					ErrInvalidDefinition, id, attached.Type)
			}
			if n.Timer == nil || n.Timer.Duration.Std() <= 0 {
				return fmt.Errorf("%w: boundary event %s missing positive timer duration", ErrInvalidDefinition, id)
			}
		case NodeExclusiveGateway:
			if len(out) < 2 {
				return fmt.Errorf("%w: exclusiveGateway %s must have ≥2 out edges", ErrInvalidDefinition, id)
			}
			defaultCount := 0
			for _, e := range out {
				if e.IsDefault || e.Condition == "default" {
					defaultCount++
				} else if e.Condition == "" {
					return fmt.Errorf("%w: exclusiveGateway %s out edge %s missing condition",
						ErrInvalidDefinition, id, e.ID)
				}
			}
			if defaultCount != 1 {
				return fmt.Errorf("%w: exclusiveGateway %s must have exactly 1 default out edge",
					ErrInvalidDefinition, id)
			}
		case NodeParallelGateway, NodeInclusiveGateway:
			isFork := len(out) >= 2
			isJoin := in >= 2
			if !isFork && !isJoin {
				return fmt.Errorf("%w: gateway %s degenerated (in=%d out=%d)",
					ErrInvalidDefinition, id, in, len(out))
			}
			if isFork && n.Type == NodeParallelGateway {
				for _, e := range out {
					if e.Condition != "" {
						return fmt.Errorf("%w: parallel fork %s out edge %s must not carry condition",
							ErrInvalidDefinition, id, e.ID)
					}
				}
			}
			if isFork && isJoin && len(out) != 1 {
				return fmt.Errorf("%w: gateway %s as join must have exactly 1 out edge",
					ErrInvalidDefinition, id)
			}
		}
	}

	if err := checkReachability(def); err != nil {
		return err
	}
	return nil
}

// checkReachability 正向 BFS 自 start 可达所有节点；自全部 end 逆向 BFS 可达所有节点。
func checkReachability(def *Definition) error {
	var start string
	ends := map[string]bool{}
	for id, n := range def.Nodes {
		switch n.Type {
		case NodeStartEvent:
			if start == "" {
				start = id
			}
		case NodeEndEvent:
			ends[id] = true
		}
	}

	reachable := bfs(start, func(id string) []string {
		var to []string
		for _, e := range def.OutEdges(id) {
			to = append(to, e.To)
		}
		return to
	})
	// 边界事件不在主流程上（无入边），以其为种子继续扩散——
	// 保证升级分支上的节点也在可达集合内。
	for id, n := range def.Nodes {
		if n.Type == NodeBoundaryEvent && !reachable[id] {
			reachable[id] = true
			queue := []string{id}
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				for _, e := range def.OutEdges(cur) {
					if !reachable[e.To] {
						reachable[e.To] = true
						queue = append(queue, e.To)
					}
				}
			}
		}
	}
	for id := range def.Nodes {
		if !reachable[id] {
			return fmt.Errorf("%w: node %s not reachable from start", ErrInvalidDefinition, id)
		}
	}

	// 逆向邻接表。
	reverse := map[string][]string{}
	for _, e := range def.Edges {
		reverse[e.To] = append(reverse[e.To], e.From)
	}
	backReach := bfs("", func(id string) []string {
		return reverse[id]
	})
	// 多终点：从所有 end 同时出发。
	queue := make([]string, 0, len(ends))
	for id := range ends {
		queue = append(queue, id)
	}
	visited := map[string]bool{}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		backReach[cur] = true
		queue = append(queue, reverse[cur]...)
	}
	for id := range def.Nodes {
		if !backReach[id] {
			return fmt.Errorf("%w: node %s cannot reach any end", ErrInvalidDefinition, id)
		}
	}
	return nil
}

func bfs(seedID string, next func(string) []string) map[string]bool {
	reached := map[string]bool{}
	if seedID == "" {
		return reached
	}
	queue := []string{seedID}
	reached[seedID] = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nx := range next(cur) {
			if !reached[nx] {
				reached[nx] = true
				queue = append(queue, nx)
			}
		}
	}
	return reached
}
