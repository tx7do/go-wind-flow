import { useEffect, useMemo, useRef, useState, forwardRef, useImperativeHandle } from 'react';
import {
  documentToTree,
  treeAddBranch,
  treeAddTaskAfter,
  treeRemoveNode,
  treeToDocument,
  treeUpdateNode,
  UnsupportedError,
  applyTheme,
  applyAccent,
  type ApprovalTree as ApprovalTreeT,
  type ApprovalTreeBranch,
  type ApprovalTreeNode,
  type DesignerDocument,
  type DesignerNode,
  type ApproverRule,
  type Theme,
} from '@go-wind/flow-designer-core';

export interface ApprovalTreeProps {
  document: DesignerDocument;
  theme?: Theme;
  accent?: string;
  onChange?: (doc: DesignerDocument) => void;
  onUnsupported?: (reason: string) => void;
  height?: string;
}

export interface ApprovalTreeRef {
  getDocument(): DesignerDocument | null;
}

type Row =
  | { key: string; depth: number; kind: 'node'; node: DesignerNode }
  | { key: string; depth: number; kind: 'branch-head'; branch: ApprovalTreeBranch; gateId: string };

export const ApprovalTree = forwardRef<ApprovalTreeRef, ApprovalTreeProps>(function ApprovalTree(
  { document: doc, theme, accent, onChange, onUnsupported, height },
  ref,
) {
  const [tree, setTree] = useState<ApprovalTreeT | null>(() => {
    try {
      return documentToTree(doc);
    } catch (err) {
      if (err instanceof UnsupportedError) onUnsupported?.(err.message);
      return null;
    }
  });
  const [selectedId, setSelectedId] = useState('');
  const [version, setVersion] = useState(0);

  const rootRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    if (rootRef.current) {
      if (theme) applyTheme(rootRef.current, theme);
      if (accent) applyAccent(rootRef.current, accent);
    }
  }, [theme, accent]);

  const commit = (t: ApprovalTreeT | null) => {
    setTree(t);
    setVersion((v) => v + 1);
    if (t && onChange) onChange(treeToDocument(t));
  };

  const rows = useMemo<Row[]>(() => {
    void version;
    const out: Row[] = [];
    let depth = 0;
    const walk = (n: ApprovalTreeNode | null) => {
      let cur = n;
      while (cur) {
        if (cur.kind === 'task') {
          out.push({ key: cur.node.id, depth, kind: 'node', node: cur.node });
          depth++;
          cur = cur.child;
          continue;
        }
        const first = cur.branches[0] ?? { id: cur.gateId + '_b0', name: '分支' };
        out.push({ key: cur.gateId + ':branches', depth, kind: 'branch-head', branch: first, gateId: cur.gateId });
        for (const b of cur.branches) {
          depth++;
          walk(b.child);
          depth--;
        }
        cur = cur.child;
      }
      depth = Math.max(0, depth - 1);
    };
    walk(tree?.root ?? null);
    return out;
  }, [tree, version]);

  const findNode = (id: string): DesignerNode | null => {
    let cur = tree?.root ?? null;
    while (cur) {
      if (cur.kind === 'task') {
        if (cur.node.id === id) return cur.node;
        cur = cur.child;
        continue;
      }
      for (const b of cur.branches) {
        const hit = findDeep(b.child, id);
        if (hit) return hit;
      }
      cur = cur.child;
    }
    return null;
  };
  function findDeep(n: ApprovalTreeNode | null, id: string): DesignerNode | null {
    let cur = n;
    while (cur) {
      if (cur.kind === 'task') {
        if (cur.node.id === id) return cur.node;
        cur = cur.child;
        continue;
      }
      for (const b of cur.branches) {
        const hit = findDeep(b.child, id);
        if (hit) return hit;
      }
      cur = cur.child;
    }
    return null;
  }

  const nodeRules = (id: string): ApproverRule[] => findNode(id)?.approver?.rules ?? [];
  const setRules = (id: string, rules: ApproverRule[]) => {
    const n = findNode(id);
    if (tree) commit(treeUpdateNode(tree, id, { approver: { ...(n?.approver ?? {}), rules } }));
  };
  const ruleKinds = ['USER', 'STARTER', 'LEADER', 'POSITION', 'ROLE', 'GROUP', 'EXPR'];

  const metaOf = (node: DesignerNode): string => {
    const parts: string[] = [];
    const rules = node.approver?.rules ?? [];
    if (rules.length) {
      parts.push(rules.map((r) => (r.kind === 'USER' || r.kind === 'ROLE' ? `${r.kind}:${r.value}` : r.kind)).join(' / '));
    }
    if (node.approver?.strategy === 'ANY') parts.push('或签');
    if (node.approver?.sequential) parts.push('串行');
    if (node.timer?.duration) parts.push(`超时 ${node.timer.duration}`);
    return parts.join(' · ') || '未配置办理人';
  };

  const selected = selectedId ? findNode(selectedId) : null;

  return (
    <div
      ref={rootRef}
      className="gwf-designer gwf-tree"
      data-gwf-theme={theme ?? 'auto'}
      style={{ ...(accent ? { ['--gwf-accent' as string]: accent } : {}), height: height ?? 'auto', overflowY: 'auto' }}
    >
      {tree === null ? (
        <div style={{ padding: 12, color: 'var(--gwf-danger)' }}>不适用审批树模式（请使用 BPMN 画布）</div>
      ) : (
        <>
          <div className="gwf-tree-node">
            <div className="gwf-node-name">发起申请</div>
          </div>
          <div className="gwf-tree-connector" />
          {rows.map((row) => (
            <div key={row.key}>
              {row.kind === 'node' ? (
                <>
                  <div
                    className={'gwf-tree-node' + (selectedId === row.node.id ? ' selected' : '')}
                    onClick={() => setSelectedId(selectedId === row.node.id ? '' : row.node.id)}
                  >
                    <div className="gwf-node-name">{row.node.name || '审批节点'}</div>
                    <div className="gwf-node-meta">{metaOf(row.node)}</div>
                    <div className="gwf-node-actions">
                      <button
                        className="gwf-btn"
                        onClick={(e) => {
                          e.stopPropagation();
                          commit(treeAddTaskAfter(tree, row.node.id));
                        }}
                      >
                        + 节点
                      </button>
                      <button
                        className="gwf-btn"
                        style={{ color: 'var(--gwf-danger)' }}
                        onClick={(e) => {
                          e.stopPropagation();
                          commit(treeRemoveNode(tree, row.node.id));
                        }}
                      >
                        删除
                      </button>
                    </div>
                    {selectedId === row.node.id ? (
                      <div className="gwf-form" onClick={(e) => e.stopPropagation()}>
                        <label>
                          节点名称
                          <input
                            value={row.node.name ?? ''}
                            onChange={(e) => commit(treeUpdateNode(tree, row.node.id, { name: e.target.value }))}
                          />
                        </label>
                        <label>办理人规则</label>
                        {nodeRules(row.node.id).map((r, i) => (
                          <div className="gwf-row" key={i}>
                            <select
                              value={r.kind}
                              onChange={(e) =>
                                setRules(row.node.id, nodeRules(row.node.id).map((x, j) => (j === i ? { ...x, kind: e.target.value as ApproverRule['kind'] } : x)))
                              }
                            >
                              {ruleKinds.map((k) => (
                                <option key={k} value={k}>
                                  {k}
                                </option>
                              ))}
                            </select>
                            <input
                              value={r.value ?? ''}
                              placeholder="编码 / 留空"
                              onChange={(e) =>
                                setRules(row.node.id, nodeRules(row.node.id).map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))
                              }
                            />
                            <button
                              className="gwf-btn"
                              style={{ color: 'var(--gwf-danger)' }}
                              onClick={() => setRules(row.node.id, nodeRules(row.node.id).filter((_, j) => j !== i))}
                            >
                              ×
                            </button>
                          </div>
                        ))}
                        <button className="gwf-btn" onClick={() => setRules(row.node.id, [...nodeRules(row.node.id), { kind: 'USER', value: '' }])}>
                          + 规则
                        </button>
                        <label>
                          审批策略
                          <select
                            value={row.node.approver?.strategy ?? 'ALL'}
                            onChange={(e) => commit(treeUpdateNode(tree, row.node.id, { approver: { ...row.node.approver, strategy: e.target.value as 'ALL' | 'ANY' } }))}
                          >
                            <option value="ALL">会签</option>
                            <option value="ANY">或签</option>
                          </select>
                        </label>
                        <label>
                          任务超时
                          <input
                            value={row.node.timer?.duration ?? ''}
                            placeholder="如 24h"
                            onChange={(e) => commit(treeUpdateNode(tree, row.node.id, { timer: { ...row.node.timer, duration: e.target.value } }))}
                          />
                        </label>
                        <label>
                          超时动作
                          <select
                            value={row.node.timer?.action ?? ''}
                            onChange={(e) => commit(treeUpdateNode(tree, row.node.id, { timer: { ...row.node.timer, action: e.target.value as '' | 'auto_approve' | 'auto_reject' } }))}
                          >
                            <option value="">仅广播事件</option>
                            <option value="auto_approve">自动通过</option>
                            <option value="auto_reject">自动驳回</option>
                          </select>
                        </label>
                      </div>
                    ) : null}
                  </div>
                  <div className="gwf-tree-connector" />
                </>
              ) : (
                <>
                  <div className="gwf-tree-branch" style={{ marginLeft: row.depth * 8 }}>
                    <div className="gwf-branch-head">
                      <span className="gwf-badge">{row.branch.isDefault ? '默认' : '条件'}</span>
                      {row.branch.isDefault ? (
                        <input value={row.branch.name} readOnly />
                      ) : (
                        <input
                          value={row.branch.condition ?? ''}
                          placeholder="条件表达式，如 amount > 1000"
                          onChange={(e) => {
                            const t = clone(tree);
                            for (const gate of allGates(t)) {
                              const b = gate.branches.find((x) => x.id === row.branch.id);
                              if (b) {
                                b.condition = e.target.value;
                                break;
                              }
                            }
                            commit(t);
                          }}
                        />
                      )}
                      <button className="gwf-btn" onClick={() => commit(treeAddBranch(tree, row.gateId))}>
                        + 分支
                      </button>
                    </div>
                  </div>
                </>
              )}
            </div>
          ))}
          <div className="gwf-tree-add">
            <button
              className="gwf-btn"
              onClick={() => {
                const last = rows.filter((r) => r.kind === "node").slice(-1)[0];
                if (last && last.kind === 'node') commit(treeAddTaskAfter(tree, last.node.id));
              }}
            >
              + 审批节点
            </button>
          </div>
          {selected ? null : null}
        </>
      )}
    </div>
  );
});

function clone(tree: ApprovalTreeT): ApprovalTreeT {
  return structuredClone(tree);
}

function allGates(tree: ApprovalTreeT): { gateId: string; branches: ApprovalTreeBranch[] }[] {
  const gates: { gateId: string; branches: ApprovalTreeBranch[] }[] = [];
  const seen = new Set<string>();
  const stack: ApprovalTreeNode[] = tree.root ? [tree.root] : [];
  while (stack.length) {
    const node = stack.pop()!;
    if (node.kind === 'branches') {
      gates.push({ gateId: node.gateId, branches: node.branches });
      for (const b of node.branches) if (b.child) stack.push(b.child);
      if (node.child) stack.push(node.child);
    } else if (node.child) {
      stack.push(node.child);
    }
  }
  return gates;
}
