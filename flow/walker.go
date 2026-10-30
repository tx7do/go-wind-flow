package flow

import (
	"context"
	"fmt"
)

// runToken 推进令牌直至阻塞（用户任务/子流程/汇合等待）或实例终结。
// 所有状态变更即时落库（持久化驱动状态机）。
func (e *Engine) runToken(ctx context.Context, inst *Instance, tk *Token, def *Definition) error {
	for step := 0; step < e.maxSteps; step++ {
		node := def.Nodes[tk.NodeID]
		if node == nil {
			return fmt.Errorf("token %s at unknown node %q", tk.ID, tk.NodeID)
		}

		switch node.Type {
		case NodeStartEvent:
			out := def.OutEdges(tk.NodeID)
			if len(out) != 1 {
				return fmt.Errorf("start %s has %d out edges", tk.NodeID, len(out))
			}
			if err := e.moveToken(ctx, inst, tk, out[0]); err != nil {
				return err
			}

		case NodeEndEvent:
			return e.finishTokenAtEnd(ctx, inst, tk)

		case NodeUserTask:
			return e.arriveAtUserTask(ctx, inst, tk, node, def)

		case NodeScriptTask, NodeServiceTask:
			if err := e.execScriptTask(ctx, inst, node); err != nil {
				return err
			}
			out := def.OutEdges(tk.NodeID)
			if len(out) != 1 {
				return fmt.Errorf("script task %s must have 1 out edge", tk.NodeID)
			}
			if err := e.moveToken(ctx, inst, tk, out[0]); err != nil {
				return err
			}

		case NodeExclusiveGateway:
			edge, err := e.selectExclusive(ctx, inst, def, node)
			if err != nil {
				return err
			}
			if err := e.moveToken(ctx, inst, tk, edge); err != nil {
				return err
			}

		case NodeParallelGateway, NodeInclusiveGateway:
			if err := e.walkGateway(ctx, inst, tk, def, node); err != nil {
				return err
			}
			if tk.Status != TokenRunning {
				return nil // 在汇合点等待
			}

		case NodeTimerEvent:
			// 中间定时器：令牌阻塞至到期；宿主 cron 驱动 ResumeDueTimers 续跑。
			now := e.clock.Now()
			due := now.Add(node.Timer.Duration.Std())
			tk.Status = TokenWaiting
			tk.DueAt = &due
			if err := e.store.Tokens().Update(ctx, tk); err != nil {
				return err
			}
			_ = e.appendHistory(ctx, inst, &Event{Type: EventTokenMoved, NodeID: tk.NodeID}, Vars{"dueAt": due.Unix()})
			return nil

		case NodeCallActivity:
			return e.startSubprocess(ctx, inst, tk, node)

		default:
			return fmt.Errorf("node %s unsupported type %q", tk.NodeID, node.Type)
		}
	}
	return fmt.Errorf("token %s exceeded max steps (%d); cyclic definition?", tk.ID, e.maxSteps)
}

// moveToken 沿边移动令牌并落库。
func (e *Engine) moveToken(ctx context.Context, inst *Instance, tk *Token, edge *Edge) error {
	tk.NodeID = edge.To
	tk.Status = TokenRunning
	if err := e.store.Tokens().Update(ctx, tk); err != nil {
		return err
	}
	_ = e.appendHistory(ctx, inst, &Event{Type: EventTokenMoved, NodeID: edge.To}, Vars{"edge": edge.ID})
	return nil
}

// finishTokenAtEnd 令牌到达终点：若实例再无活令牌则终结实例（APPROVED）。
func (e *Engine) finishTokenAtEnd(ctx context.Context, inst *Instance, tk *Token) error {
	tk.Status = TokenEnded
	if err := e.store.Tokens().Update(ctx, tk); err != nil {
		return err
	}
	tokens, err := e.store.Tokens().ListByInstance(ctx, inst.ID)
	if err != nil {
		return err
	}
	for _, t := range tokens {
		if t.ID != tk.ID && t.Status != TokenEnded {
			return nil // 还有并行分支在途
		}
	}
	// 走到终点 = 实例正常通过。
	now := e.clock.Now()
	inst.Status = InstanceApproved
	inst.EndedAt = &now
	if err := e.store.Instances().Update(ctx, inst); err != nil {
		return err
	}
	_ = e.appendHistory(ctx, inst, &Event{Type: EventProcessEnded, Status: InstanceApproved}, nil)
	e.fireListeners(ctx, &Event{
		Type: EventProcessEnded, TenantID: inst.TenantID, InstanceID: inst.ID,
		Status: InstanceApproved, Vars: inst.Variables.Clone(), At: e.clock.Now(),
	})
	if inst.ParentInstanceID != "" {
		return e.resumeParentOnChildEnd(ctx, inst, InstanceApproved)
	}
	return nil
}

// arriveAtUserTask 到达用户任务：解析办理人、创建任务、令牌阻塞。
func (e *Engine) arriveAtUserTask(ctx context.Context, inst *Instance, tk *Token, node *Node, def *Definition) error {
	assignees, err := e.resolveApprovers(ctx, inst, node)
	if err != nil {
		// fail-closed：解析失败不创建任务，令牌留在原地（RUNNING），
		// 由宿主决定重试或终止。
		return err
	}

	now := e.clock.Now()
	tk.Status = TokenWaiting
	if err := e.store.Tokens().Update(ctx, tk); err != nil {
		return err
	}

	for _, uid := range assignees {
		task := &Task{
			ID:         e.idgen(),
			InstanceID: inst.ID,
			TenantID:   inst.TenantID,
			NodeID:     node.ID,
			TokenID:    tk.ID,
			Name:       node.Name,
			Assignee:   uid,
			Status:     TaskPending,
			CreatedAt:  now,
		}
		if node.Timer != nil {
			due := now.Add(node.Timer.Duration.Std())
			task.DueAt = &due
		}
		if err := e.store.Tasks().Save(ctx, task); err != nil {
			return err
		}
		_ = e.appendHistory(ctx, inst, &Event{
			Type: EventTaskCreated, NodeID: node.ID, TaskID: task.ID,
		}, Vars{"assignee": uid})
		e.fireListeners(ctx, &Event{
			Type: EventTaskCreated, TenantID: inst.TenantID, InstanceID: inst.ID,
			TokenID: tk.ID, TaskID: task.ID, NodeID: node.ID,
			ActorID: uid, Vars: inst.Variables.Clone(), At: now,
		})
	}
	return nil
}

// resolveApprovers 解析任务办理人：去重保序 + 委托叠加。
func (e *Engine) resolveApprovers(ctx context.Context, inst *Instance, node *Node) ([]string, error) {
	if node.Approver == nil || len(node.Approver.Rules) == 0 {
		return nil, fmt.Errorf("%w: node %s has no approver rules", ErrInvalidDefinition, node.ID)
	}

	needDynamic := false
	for _, r := range node.Approver.Rules {
		switch r.Kind {
		case ApproverLeader, ApproverPosition, ApproverRole, ApproverExpr:
			needDynamic = true
		}
	}

	var ids []string
	switch {
	case e.identity != nil:
		resolved, err := e.identity.ResolveApprovers(ctx, ResolveRequest{
			TenantID:     inst.TenantID,
			InstanceID:   inst.ID,
			DefinitionID: inst.DefinitionKey,
			NodeID:       node.ID,
			StarterID:    inst.StarterID,
			InstanceVars: inst.Variables,
			Rules:        node.Approver.Rules,
		})
		if err != nil {
			return nil, err
		}
		ids = resolved
	case !needDynamic:
		// 内置静态解析：USER / STARTER。
		seen := map[string]bool{}
		for _, r := range node.Approver.Rules {
			v := r.Value
			if r.Kind == ApproverStarter {
				v = inst.StarterID
			}
			if v != "" && !seen[v] {
				seen[v] = true
				ids = append(ids, v)
			}
		}
	default:
		return nil, fmt.Errorf("%w: node %s needs dynamic approver but IdentityProvider is nil",
			ErrNotConfigured, node.ID)
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("node %s resolved to zero approvers (fail-closed)", node.ID)
	}

	// 委托叠加：受托人代办。
	if e.identity != nil {
		for i, uid := range ids {
			delegate, err := e.identity.ResolveDelegate(ctx, inst.TenantID, uid)
			if err != nil {
				e.log.Printf("flow: resolve delegation for %s failed: %v (keep original)", uid, err)
				continue
			}
			ids[i] = delegate
		}
	}
	return ids, nil
}

// execScriptTask 执行脚本任务；返回 map 时把键值合并进实例变量。
func (e *Engine) execScriptTask(ctx context.Context, inst *Instance, node *Node) error {
	if node.Script == nil || node.Script.Code == "" {
		return fmt.Errorf("%w: script task %s has no script", ErrInvalidDefinition, node.ID)
	}
	if e.scripts == nil {
		return fmt.Errorf("%w: ScriptRunner is nil (node %s)", ErrNotConfigured, node.ID)
	}
	result, err := e.scripts.Run(ctx, node.Script, inst.Variables.Clone())
	if err != nil {
		_ = e.appendHistory(ctx, inst, &Event{Type: EventTokenMoved, NodeID: node.ID}, Vars{"scriptError": err.Error()})
		return fmt.Errorf("%w: node %s: %v", ErrScript, node.ID, err)
	}
	switch m := result.(type) {
	case map[string]any:
		if len(m) > 0 {
			inst.Variables.Merge(m)
			if err := e.store.Instances().Update(ctx, inst); err != nil {
				return err
			}
			_ = e.appendHistory(ctx, inst, &Event{Type: EventVariableChanged}, m)
		}
	case Vars:
		if len(m) > 0 {
			inst.Variables.Merge(m)
			if err := e.store.Instances().Update(ctx, inst); err != nil {
				return err
			}
			_ = e.appendHistory(ctx, inst, &Event{Type: EventVariableChanged}, m)
		}
	}
	return nil
}

// selectExclusive 排他网关选边：按定义顺序求值，命中即走；全不中走默认；无默认 fail-closed。
func (e *Engine) selectExclusive(ctx context.Context, inst *Instance, def *Definition, node *Node) (*Edge, error) {
	var defEdge *Edge
	for _, edge := range def.OutEdges(node.ID) {
		if edge.IsDefault || edge.Condition == "default" {
			defEdge = edge
			continue
		}
		ok, err := e.evaluator.EvalBool(ctx, edge.Condition, inst.Variables)
		if err != nil {
			return nil, fmt.Errorf("%w: gateway %s edge %s: %v (fail-closed)", ErrExpression, node.ID, edge.ID, err)
		}
		if ok {
			_ = e.appendHistory(ctx, inst, &Event{Type: EventGatewayDecided, NodeID: node.ID}, Vars{"edge": edge.ID})
			return edge, nil
		}
	}
	if defEdge == nil {
		return nil, fmt.Errorf("%w: gateway %s no branch matched and no default edge", ErrExpression, node.ID)
	}
	_ = e.appendHistory(ctx, inst, &Event{Type: EventGatewayDecided, NodeID: node.ID}, Vars{"edge": defEdge.ID})
	return defEdge, nil
}

// walkGateway 并行/包容网关：汇合（in>=2）→ 等待或吸收；分叉（out>=2）→ 生成分支令牌。
func (e *Engine) walkGateway(ctx context.Context, inst *Instance, tk *Token, def *Definition, node *Node) error {
	in := def.InDegree(node.ID)
	out := def.OutEdges(node.ID)

	// ---- 汇合 ----
	if in >= 2 {
		need := in
		if node.Type == NodeInclusiveGateway {
			need = int(joinExpected(inst.Variables, node.ID))
			if need < 1 {
				need = in
			}
		}
		tk.Status = TokenWaiting
		if err := e.store.Tokens().Update(ctx, tk); err != nil {
			return err
		}
		tokens, err := e.store.Tokens().ListByInstance(ctx, inst.ID)
		if err != nil {
			return err
		}
		var waiting []*Token
		for _, t := range tokens {
			if t.NodeID == node.ID && t.Status == TokenWaiting {
				waiting = append(waiting, t)
			}
		}
		if len(waiting) < need {
			return nil // 等待其他分支到达
		}
		// 吸收：保留本令牌，清除其余等待令牌。
		for _, t := range waiting {
			if t.ID != tk.ID {
				if err := e.store.Tokens().Delete(ctx, t.ID); err != nil {
					return err
				}
			}
		}
		tk.Status = TokenRunning
		if err := e.store.Tokens().Update(ctx, tk); err != nil {
			return err
		}
		_ = e.appendHistory(ctx, inst, &Event{Type: EventGatewayDecided, NodeID: node.ID}, Vars{"join": node.ID})
	}

	// ---- 分叉 ----
	if len(out) >= 2 {
		if node.Type == NodeInclusiveGateway {
			var chosen []*Edge
			var defEdge *Edge
			for _, edge := range out {
				if edge.IsDefault || edge.Condition == "" {
					// 无条件出边视作默认分支（仅当全部条件分支未命中时走）。
					if defEdge == nil {
						defEdge = edge
					}
					continue
				}
				ok, err := e.evaluator.EvalBool(ctx, edge.Condition, inst.Variables)
				if err != nil {
					return fmt.Errorf("%w: inclusive gateway %s edge %s: %v", ErrExpression, node.ID, edge.ID, err)
				}
				if ok {
					chosen = append(chosen, edge)
				}
			}
			if len(chosen) == 0 {
				if defEdge == nil {
					return fmt.Errorf("%w: inclusive gateway %s no branch matched and no default", ErrExpression, node.ID)
				}
				chosen = []*Edge{defEdge}
			}
			out = chosen
			inst.Variables[joinExpectedKey(node.ID)] = float64(len(out))
			if err := e.store.Instances().Update(ctx, inst); err != nil {
				return err
			}
		}

		// 本令牌走第一条分支，其余分支各生成一个子令牌。
		for i, edge := range out {
			if i == 0 {
				if err := e.moveToken(ctx, inst, tk, edge); err != nil {
					return err
				}
				continue
			}
			child := &Token{
				ID:         e.idgen(),
				InstanceID: inst.ID,
				NodeID:     edge.To,
				Status:     TokenRunning,
				ParentID:   tk.ID,
				CreatedAt:  e.clock.Now(),
			}
			if err := e.store.Tokens().Save(ctx, child); err != nil {
				return err
			}
			if err := e.runToken(ctx, inst, child, def); err != nil {
				return err
			}
		}
		return nil
	}

	// 普通直通（汇合后的唯一出边）。
	if len(out) == 1 {
		return e.moveToken(ctx, inst, tk, out[0])
	}
	return fmt.Errorf("gateway %s has no out edge", node.ID)
}

// startSubprocess 发起子流程（callActivity），令牌阻塞等待子实例终结。
func (e *Engine) startSubprocess(ctx context.Context, inst *Instance, tk *Token, node *Node) error {
	tk.Status = TokenWaiting
	if err := e.store.Tokens().Update(ctx, tk); err != nil {
		return err
	}
	child, _, err := e.Start(ctx, StartRequest{
		TenantID:         inst.TenantID,
		Key:              node.SubprocessKey,
		Version:          node.SubprocessVersion,
		BusinessKey:      inst.BusinessKey,
		BusinessType:     inst.BusinessType,
		StarterID:        inst.StarterID,
		Variables:        inst.Variables.Clone(),
		ParentInstanceID: inst.ID,
		ParentTokenID:    tk.ID,
	})
	if err != nil {
		return fmt.Errorf("start subprocess %s v%d: %w", node.SubprocessKey, node.SubprocessVersion, err)
	}
	// 若子流程同步终结（全程自动节点），resumeParentOnChildEnd 已在
	// 子实例终结链路里恢复本令牌推进，这里不能再用旧 tk 覆盖状态。
	if child.Status.Live() {
		tk.ChildInstID = child.ID
		if err := e.store.Tokens().Update(ctx, tk); err != nil {
			return err
		}
	}
	return nil
}

func joinExpectedKey(nodeID string) string { return "flow:joinExpected:" + nodeID }

func joinExpected(vars Vars, nodeID string) float64 {
	v, _ := vars[joinExpectedKey(nodeID)].(float64)
	return v
}
