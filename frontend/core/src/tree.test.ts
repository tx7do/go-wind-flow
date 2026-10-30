import { describe, expect, it } from 'vitest';
import {
  documentToTree,
  emptyTree,
  treeAddBranch,
  treeAddTaskAfter,
  treeRemoveNode,
  treeToDocument,
  treeUpdateNode,
  UnsupportedError,
} from './tree';
import type { DesignerDocument } from './types';

const linearDoc: DesignerDocument = {
  version: 1,
  key: 't',
  name: '测试',
  nodes: [
    { id: 'start', type: 'startEvent' },
    { id: 'a', type: 'userTask', name: 'A', approver: { rules: [{ kind: 'LEADER' }], strategy: 'ALL' } },
    { id: 'b', type: 'userTask', name: 'B', approver: { rules: [{ kind: 'USER', value: 'u2' }], strategy: 'ANY' } },
    { id: 'end', type: 'endEvent' },
  ],
  edges: [
    { id: 'e1', source: 'start', target: 'a' },
    { id: 'e2', source: 'a', target: 'b' },
    { id: 'e3', source: 'b', target: 'end' },
  ],
};

const branchDoc: DesignerDocument = {
  version: 1,
  key: 't',
  nodes: [
    { id: 'start', type: 'startEvent' },
    { id: 'a', type: 'userTask', approver: { rules: [{ kind: 'LEADER' }] } },
    { id: 'split', type: 'exclusiveGateway', defaultEdge: 'bd' },
    { id: 'hi', type: 'userTask', approver: { rules: [{ kind: 'USER', value: 'u_hi' }] } },
    { id: 'lo', type: 'userTask', approver: { rules: [{ kind: 'USER', value: 'u_lo' }] } },
    { id: 'merge', type: 'exclusiveGateway' },
    { id: 'end', type: 'endEvent' },
  ],
  edges: [
    { id: 'e1', source: 'start', target: 'a' },
    { id: 'e2', source: 'a', target: 'split' },
    { id: 'bh', source: 'split', target: 'hi', name: '高额', condition: 'amount > 1000' },
    { id: 'bd', source: 'split', target: 'lo', isDefault: true },
    { id: 'mh', source: 'hi', target: 'merge' },
    { id: 'md', source: 'lo', target: 'merge' },
    { id: 'me', source: 'merge', target: 'end' },
  ],
};

describe('documentToTree', () => {
  it('解析线性链', () => {
    const tree = documentToTree(linearDoc);
    expect(tree.root?.kind).toBe('task');
    if (tree.root?.kind === 'task') {
      expect(tree.root.node.id).toBe('a');
      expect(tree.root.child?.kind).toBe('task');
      if (tree.root.child?.kind === 'task') expect(tree.root.child.node.id).toBe('b');
      expect(tree.root.child?.child).toBeNull();
    }
  });

  it('解析分支构造（拆分/汇合/默认分支）', () => {
    const tree = documentToTree(branchDoc);
    expect(tree.root?.kind).toBe('task');
    if (tree.root?.kind === 'task') {
      expect(tree.root.node.id).toBe('a');
      const gate = tree.root.child;
      expect(gate?.kind).toBe('branches');
      if (gate?.kind === 'branches') {
        expect(gate.branches).toHaveLength(2);
        expect(gate.branches[0].condition).toBe('amount > 1000');
        expect(gate.branches[1].isDefault).toBe(true);
        expect(gate.branches[0].child?.kind).toBe('task');
      }
    }
  });

  it('子集外节点报 UnsupportedError', () => {
    const doc: DesignerDocument = {
      ...linearDoc,
      nodes: [...linearDoc.nodes, { id: 'pg', type: 'parallelGateway' }],
      edges: [...linearDoc.edges, { id: 'e4', source: 'pg', target: 'end' }],
    };
    expect(() => documentToTree(doc)).toThrow(UnsupportedError);
  });
});

describe('treeToDocument', () => {
  it('线性链回环（结构保持）', () => {
    const tree = documentToTree(linearDoc);
    const doc = treeToDocument(tree);
    const re = documentToTree(doc);
    const ids = (t: typeof re) => {
      const out: string[] = [];
      let cur = t.root;
      while (cur) {
        if (cur.kind === 'task') {
          out.push(cur.node.id);
          cur = cur.child;
        } else {
          out.push(cur.gateId);
          cur = cur.child;
        }
      }
      return out;
    };
    expect(ids(re)).toEqual(['a', 'b']);
    expect(re.root?.kind === 'task' && re.root.node.approver?.strategy).toBe('ALL');
    if (re.root?.kind === 'task' && re.root.child?.kind === 'task') {
      expect(re.root.child.node.approver?.strategy).toBe('ANY');
    }
  });

  it('分支回环（条件/默认/汇合保持）', () => {
    const tree = documentToTree(branchDoc);
    const doc = treeToDocument(tree);
    const re = documentToTree(doc);
    expect(re.root?.kind).toBe('task');
    if (re.root?.kind === 'task') {
      const gate = re.root.child;
      expect(gate?.kind).toBe('branches');
      if (gate?.kind === 'branches') {
        expect(gate.branches[0].condition).toBe('amount > 1000');
        expect(gate.branches[1].isDefault).toBe(true);
        expect(gate.branches[0].child?.kind).toBe('task');
      }
    }
  });
});

describe('树操作', () => {
  it('插入/更新/删除', () => {
    const tree = documentToTree(linearDoc);
    const withNew = treeAddTaskAfter(tree, 'a', { name: 'C' });
    let cur = withNew.root;
    const ids: string[] = [];
    while (cur) {
      if (cur.kind === 'task') ids.push(cur.node.id);
      cur = cur.child;
    }
    expect(ids).toEqual(['a', 'task_1', 'b']);

    const updated = treeUpdateNode(withNew, 'task_1', { name: '复核' });
    expect(JSON.stringify(updated)).toContain('复核');

    const removed = treeRemoveNode(updated, 'task_1');
    const after = documentToTree(treeToDocument(removed));
    let cur2 = after.root;
    const ids2: string[] = [];
    while (cur2) {
      if (cur2.kind === 'task') ids2.push(cur2.node.id);
      cur2 = cur2.child;
    }
    expect(ids2).toEqual(['a', 'b']);
  });

  it('加分支', () => {
    const tree = documentToTree(branchDoc);
    const gateId = tree.root?.kind === 'task' && tree.root.child?.kind === 'branches' ? tree.root.child.gateId : '';
    const withBranch = treeAddBranch(tree, gateId);
    const doc = treeToDocument(withBranch);
    const re = documentToTree(doc);
    const gate = re.root?.kind === 'task' ? re.root.child : null;
    expect(gate?.kind === 'branches' ? gate.branches.length : 0).toBe(3);
  });

  it('emptyTree 起步', () => {
    const tree = emptyTree('new_flow', '新流程');
    expect(tree.root?.kind).toBe('task');
    const doc = treeToDocument(tree);
    expect(doc.nodes.length).toBeGreaterThanOrEqual(3);
  });
});
import { treeAddTaskFirst, treeAppendTaskToBranch, treeRemoveBranch } from './tree';

describe('分支级操作', () => {
  it('追加任务到分支链 / 删除分支并解包', () => {
    const tree = documentToTree(branchDoc);
    const gateId = tree.root?.kind === 'task' && tree.root.child?.kind === 'branches' ? tree.root.child.gateId : '';

    // 追加到高额分支链尾。
    const appended = treeAppendTaskToBranch(tree, 'bh', { name: '追加审批' });
    const doc1 = treeToDocument(appended);
    expect(doc1.nodes.some((n) => n.name === '追加审批')).toBe(true);
    // 回环仍合法。
    expect(() => documentToTree(doc1)).not.toThrow();

    // 删除默认分支 → 只剩一条分支 → 解包（分支构造消失）。
    const removed = treeRemoveBranch(appended, gateId, 'bd');
    const doc2 = treeToDocument(removed);
    const gateNode = doc2.nodes.find((n) => n.id === gateId);
    expect(gateNode).toBeUndefined();
    expect(() => documentToTree(doc2)).not.toThrow();
  });

  it('前置插入任务', () => {
    const tree = documentToTree(linearDoc);
    const first = treeAddTaskFirst(tree, { name: '前置' });
    expect(first.root?.kind === 'task' && first.root.node.name).toBe('前置');
    const doc = treeToDocument(first);
    expect(() => documentToTree(doc)).not.toThrow();
  });
});
