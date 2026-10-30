/**
 * 垂直审批树（钉钉式业务模式）：designer-json-v1 子集的树形结构与互导。
 *
 * 子集（审批场景 80/20）：startEvent、userTask、exclusiveGateway（拆分+汇合）、
 * endEvent。子集之外（并行网关/消息/定时器中间事件/子流程/边界/脚本/错误终点）
 * 抛 UnsupportedError——宿主回退到 bpmn-js 画布模式。
 *
 * 树形语义（钉钉式）：
 *   线性链：审批节点依次串联；
 *   分支构造：排他拆分网关 → N 条分支链 → 汇合网关 → 后续；
 *   每条分支各自走完自己的链后在汇合点收敛。
 */
import type { DesignerDocument, DesignerEdge, DesignerNode, ApproverRule } from './types';

export class UnsupportedError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'UnsupportedError';
  }
}

export interface ApprovalTreeTask {
  kind: 'task';
  node: DesignerNode;
  child: ApprovalTreeNode | null;
}

export interface ApprovalTreeBranch {
  /** 分支边 ID（树模式生成）。 */
  id: string;
  name: string;
  condition?: string;
  isDefault?: boolean;
  /** 分支链首节点（null = 直通汇合点）。 */
  child: ApprovalTreeNode | null;
}

export interface ApprovalTreeBranches {
  kind: 'branches';
  /** 拆分网关 ID；汇合网关 ID = gateId + '_merge'。 */
  gateId: string;
  branches: ApprovalTreeBranch[];
  /** 汇合后的后续链（null = 直接到结束）。 */
  child: ApprovalTreeNode | null;
}

export type ApprovalTreeNode = ApprovalTreeTask | ApprovalTreeBranches;

export interface ApprovalTree {
  key: string;
  name?: string;
  root: ApprovalTreeNode | null;
  end: { id: string; name?: string };
}

// ============================================================
// 文档 → 树
// ============================================================

export function documentToTree(doc: DesignerDocument): ApprovalTree {
  const subset = new Set(['startEvent', 'endEvent', 'userTask', 'exclusiveGateway']);
  for (const n of doc.nodes) {
    if (!subset.has(n.type)) {
      throw new UnsupportedError(`节点 ${n.id}（${n.type}）不适用审批树模式`);
    }
  }
  const starts = doc.nodes.filter((n) => n.type === 'startEvent');
  const ends = doc.nodes.filter((n) => n.type === 'endEvent');
  if (starts.length !== 1) throw new UnsupportedError('审批树要求恰好一个开始节点');
  if (ends.length !== 1) throw new UnsupportedError('审批树要求恰好一个结束节点');

  const byId = new Map(doc.nodes.map((n) => [n.id, n]));
  const out = new Map<string, DesignerEdge[]>();
  const inDeg = new Map<string, number>();
  for (const n of doc.nodes) inDeg.set(n.id, 0);
  for (const e of doc.edges) {
    if (!out.has(e.source)) out.set(e.source, []);
    out.get(e.source)!.push(e);
    inDeg.set(e.target, (inDeg.get(e.target) ?? 0) + 1);
  }

  const startId = starts[0].id;
  const endId = ends[0].id;
  const visited = new Set<string>();

  function isMergeGateway(id: string): boolean {
    const n = byId.get(id);
    return !!n && n.type === 'exclusiveGateway' && (inDeg.get(id) ?? 0) >= 2 && (out.get(id) ?? []).length === 1;
  }

  /** 找 N 条分支链的共同汇合点：各链经单出 userTask 前行，首个被全部到达的
   *  汇合网关（或全部直达的 endId）。 */
  function findMergeId(branchTargets: string[]): string {
    const pointiers = [...new Set(branchTargets)];
    if (pointiers.length === 1) {
      const id = pointiers[0];
      if (id === endId || isMergeGateway(id)) return id;
      throw new UnsupportedError('分支未收敛，不适用审批树模式');
    }
    const reached = new Map<string, Set<number>>();
    const frontier: { id: string; b: number }[] = pointiers.map((id, i) => ({ id, b: i }));
    let guard = 0;
    while (frontier.length) {
      if (++guard > 2000) throw new UnsupportedError('分支汇合解析超限');
      const cur = frontier.shift()!;
      if (cur.id === endId || isMergeGateway(cur.id)) {
        const set = reached.get(cur.id) ?? new Set<number>();
        set.add(cur.b);
        reached.set(cur.id, set);
        if (set.size === pointiers.length) return cur.id;
        continue;
      }
      const set = reached.get(cur.id) ?? new Set<number>();
      if (set.has(cur.b)) continue;
      set.add(cur.b);
      reached.set(cur.id, set);
      const node = byId.get(cur.id);
      if (!node || node.type !== 'userTask') {
        throw new UnsupportedError(`分支内节点 ${cur.id}（${node?.type ?? '?'}）不适用审批树模式`);
      }
      const outs = out.get(cur.id) ?? [];
      if (outs.length !== 1) throw new UnsupportedError(`分支内节点 ${cur.id} 多出边，不适用审批树模式`);
      frontier.push({ id: outs[0].target, b: cur.b });
    }
    throw new UnsupportedError('分支未收敛，不适用审批树模式');
  }

  /** 从 fromId 走链，遇到 stop 集合（汇合网关/end）即停。返回链 + 停点。 */
  function parseChain(fromId: string, stops: Set<string>): { chain: ApprovalTreeNode | null; stoppedAt: string } {
    const tasks: ApprovalTreeTask[] = [];
    let cur = fromId;
    while (!stops.has(cur)) {
      if (visited.has(cur)) throw new UnsupportedError('流程存在环，不适用审批树模式');
      visited.add(cur);
      const node = byId.get(cur);
      if (!node) throw new UnsupportedError(`未知节点 ${cur}`);

      // 开始节点：直达其唯一出边目标（树从开始之后的第一个节点算起）。
      if (node.type === 'startEvent') {
        const outs = out.get(cur) ?? [];
        if (outs.length !== 1) throw new UnsupportedError('开始节点出边异常');
        cur = outs[0].target;
        continue;
      }
      if (node.type === 'endEvent') {
        return { chain: link(tasks), stoppedAt: cur };
      }
      if (node.type === 'exclusiveGateway') {
        const outs = out.get(cur) ?? [];
        if (outs.length === 1 && (inDeg.get(cur) ?? 0) >= 2) {
          // 汇合网关：当前链结束（merge 的出边由外层 parseChain 续走）。
          return { chain: link(tasks), stoppedAt: cur };
        }
        if ((inDeg.get(cur) ?? 0) >= 2) {
          throw new UnsupportedError(`网关 ${cur} 形态不受支持（多入多出）`);
        }
        // 拆分：解析各分支 + 汇合点，汇合后续链并入 child。
        const branchTargets = outs.map((e) => e.target);
        const mergeId = findMergeId(branchTargets);
        const innerStops = new Set([...stops, mergeId]);
        const branches = outs.map((e) => ({
          id: e.id,
          name: e.name ?? (e.isDefault ? '默认' : '分支'),
          condition: e.condition || undefined,
          isDefault: e.isDefault || undefined,
          child: e.target === mergeId ? null : parseChain(e.target, innerStops).chain,
        }));
        if (mergeId === endId) {
          return { chain: link(tasks, { kind: 'branches', gateId: cur, branches, child: null }), stoppedAt: endId };
        }
        visited.add(mergeId);
        const mergeOut = out.get(mergeId) ?? [];
        if (mergeOut.length !== 1) throw new UnsupportedError(`汇合网关 ${mergeId} 出边异常`);
        const rest = parseChain(mergeOut[0].target, stops);
        visited.delete(mergeId); // merge 由树转回文档时重新生成
        return { chain: link(tasks, { kind: 'branches', gateId: cur, branches, child: rest.chain }), stoppedAt: rest.stoppedAt };
      }
      if (node.type !== 'userTask') {
        throw new UnsupportedError(`节点 ${cur}（${node.type}）不适用审批树模式`);
      }
      const outs = out.get(cur) ?? [];
      if (outs.length !== 1) throw new UnsupportedError(`节点 ${cur} 多出边，不适用审批树模式`);
      tasks.push({ kind: 'task', node, child: null });
      cur = outs[0].target;
    }
    return { chain: link(tasks), stoppedAt: cur };

    function link(tasks: ApprovalTreeTask[], tail?: ApprovalTreeNode | null): ApprovalTreeNode | null {
      const tailNode = tail ?? null;
      for (let i = tasks.length - 1; i >= 0; i--) tasks[i].child = i + 1 < tasks.length ? tasks[i + 1] : tailNode;
      if (tasks.length) return tasks[0];
      return tailNode;
    }
  }

  const { chain } = parseChain(startId, new Set([endId]));
  return { key: doc.key, name: doc.name, root: chain, end: { id: endId, name: ends[0].name } };
}

// ============================================================
// 树 → 文档
// ============================================================

export function treeToDocument(tree: ApprovalTree): DesignerDocument {
  const nodes: DesignerNode[] = [];
  const edges: DesignerEdge[] = [];
  let counter = 0;
  const eid = () => `e_${++counter}`;

  const endId = tree.end.id || 'end';
  nodes.push({ id: 'start', type: 'startEvent', name: '开始', ui: { x: 160, y: 160 } });
  nodes.push({ id: endId, type: 'endEvent', name: tree.end.name ?? '结束', ui: { x: 160, y: 1200 } });

  let y = 280;

  /** 输出链：接线全部内部边与链尾→targetId，返回链首节点 ID。 */
  function emitChainFrom(node: ApprovalTreeNode | null, targetId: string): string {
    if (!node) return targetId;
    if (node.kind === 'task') {
      y += 130;
      const n = { ...node.node, ui: { x: 160, y } };
      nodes.push(n);
      const restFirst = emitChainFrom(node.child, targetId);
      edges.push({ id: eid(), source: n.id, target: restFirst });
      return n.id;
    }
    // 分支构造：拆分 → 各分支（元数据边） → 汇合 → 后续。
    y += 100;
    const mergeId = node.gateId + '_merge';
    const defaultBranch = node.branches.find((b) => b.isDefault);
    nodes.push({
      id: node.gateId,
      type: 'exclusiveGateway',
      name: '条件分支',
      defaultEdge: defaultBranch?.id,
      ui: { x: 140, y },
    });
    const mergeY = y + node.branches.length * 90;
    nodes.push({ id: mergeId, type: 'exclusiveGateway', name: '汇合', ui: { x: 140, y: mergeY } });

    for (const b of node.branches) {
      const first = emitChainFrom(b.child, mergeId);
      edges.push({
        id: b.id,
        source: node.gateId,
        target: first,
        name: b.name,
        condition: b.condition,
        isDefault: b.isDefault,
      });
    }
    const after = emitChainFrom(node.child, targetId);
    edges.push({ id: eid(), source: mergeId, target: after });
    return node.gateId;
  }

  edges.push({ id: eid(), source: 'start', target: emitChainFrom(tree.root, endId) });

  return { version: 1, key: tree.key, name: tree.name, nodes, edges };
}

// ============================================================
// 树操作（纯函数：structuredClone 后修改，返回新树）
// ============================================================

export function cloneTree(tree: ApprovalTree): ApprovalTree {
  return structuredClone(tree);
}

export function emptyTree(key: string, name?: string): ApprovalTree {
  return {
    key,
    name,
    end: { id: 'end', name: '结束' },
    root: {
      kind: 'task',
      node: { id: 'task_1', type: 'userTask', name: '审批节点', approver: { rules: [{ kind: 'USER', value: '' }], strategy: 'ALL' } },
      child: null,
    },
  };
}

/** 在某任务节点后插入新审批节点。 */
export function treeAddTaskAfter(tree: ApprovalTree, afterNodeId: string, init?: Partial<DesignerNode>): ApprovalTree {
  const t = cloneTree(tree);
  const slot = findTask(t.root, afterNodeId);
  if (!slot) throw new Error(`node ${afterNodeId} not found`);
  const fresh: ApprovalTreeTask = {
    kind: 'task',
    node: {
      id: freshId(t, 'task'),
      type: 'userTask',
      name: init?.name ?? '审批节点',
      approver: init?.approver ?? { rules: [{ kind: 'USER', value: '' }], strategy: 'ALL' },
    },
    child: slot.child,
  };
  slot.child = fresh;
  return t;
}

/** 为分支构造追加分支（自动带一个空审批节点）。 */
export function treeAddBranch(tree: ApprovalTree, gateId: string): ApprovalTree {
  const t = cloneTree(tree);
  const gate = findBranches(t.root, gateId);
  if (!gate) throw new Error(`branches ${gateId} not found`);
  gate.branches.push({
    id: freshId(t, 'branch'),
    name: '分支',
    child: { kind: 'task', node: { id: freshId(t, 'task'), type: 'userTask', name: '审批节点', approver: { rules: [{ kind: 'USER', value: '' }], strategy: 'ALL' } }, child: null },
  });
  return t;
}

/** 删除任务节点（由前驱接管其 child；分支首被删则分支接管下游）。 */
export function treeRemoveNode(tree: ApprovalTree, nodeId: string): ApprovalTree {
  const t = cloneTree(tree);
  // 根即目标：根令牌位由其 child 接管。
  if (t.root && t.root.kind === 'task' && t.root.node.id === nodeId) {
    t.root = t.root.child;
    return t;
  }
  walkRemove(t.root, nodeId);
  return t;

  function walkRemove(node: ApprovalTreeNode | null, id: string): boolean {
    if (!node) return false;
    if (node.kind === 'task') {
      const c = node.child;
      if (c && c.kind === 'task' && c.node.id === id) {
        node.child = c.child; // 前驱接管
        return true;
      }
      return walkRemove(c, id);
    }
    for (const b of node.branches) {
      const bc = b.child;
      if (bc && bc.kind === 'task' && bc.node.id === id) {
        b.child = bc.child; // 分支首被删：分支接管下游
        return true;
      }
      if (walkRemove(bc, id)) return true;
    }
    const cc = node.child;
    if (cc && cc.kind === 'task' && cc.node.id === id) {
      node.child = cc.child;
      return true;
    }
    return walkRemove(cc, id);
  }
}

export function treeUpdateNode(tree: ApprovalTree, nodeId: string, patch: Partial<DesignerNode>): ApprovalTree {
  const t = cloneTree(tree);
  const slot = findTask(t.root, nodeId);
  if (!slot) throw new Error(`node ${nodeId} not found`);
  slot.node = { ...slot.node, ...patch, id: slot.node.id, type: slot.node.type };
  return t;
}

function findTask(root: ApprovalTreeNode | null, id: string): ApprovalTreeTask | null {
  let cur = root;
  while (cur) {
    if (cur.kind === 'task') {
      if (cur.node.id === id) return cur;
      cur = cur.child;
      continue;
    }
    for (const b of cur.branches) {
      const hit = findTask(b.child, id);
      if (hit) return hit;
    }
    cur = cur.child;
  }
  return null;
}

function findBranches(root: ApprovalTreeNode | null, gateId: string): ApprovalTreeBranches | null {
  let cur = root;
  while (cur) {
    if (cur.kind === 'branches') {
      if (cur.gateId === gateId) return cur;
      for (const b of cur.branches) {
        const hit = findBranches(b.child, gateId);
        if (hit) return hit;
      }
      cur = cur.child;
      continue;
    }
    cur = cur.child;
  }
  return null;
}

function freshId(tree: ApprovalTree, prefix: string): string {
  const ids = new Set<string>();
  const collect = (n: ApprovalTreeNode | null) => {
    let cur = n;
    while (cur) {
      if (cur.kind === 'task') {
        ids.add(cur.node.id);
        cur = cur.child;
      } else {
        ids.add(cur.gateId);
        for (const b of cur.branches) {
          ids.add(b.id);
          collect(b.child);
        }
        cur = cur.child;
      }
    }
  };
  collect(tree.root);
  ids.add(tree.end.id);
  let i = 1;
  while (ids.has(`${prefix}_${i}`)) i++;
  return `${prefix}_${i}`;
}

export type { ApproverRule };
