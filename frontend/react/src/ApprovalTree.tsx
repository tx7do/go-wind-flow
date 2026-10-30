import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  forwardRef,
  useImperativeHandle,
} from 'react';
import {
  documentToTree,
  treeAddBranch,
  treeAddTaskAfter,
  treeAddTaskAfterBranches,
  treeAddTaskFirst,
  treeAppendTaskToBranch,
  treeRemoveBranch,
  treeRemoveNode,
  treeToDocument,
  treeUpdateNode,
  UnsupportedError,
  applyTheme,
  applyAccent,
  ensureStyles,
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

interface TreeApi {
  tree: ApprovalTreeT;
  selectedId: string;
  select(id: string): void;
  commit(t: ApprovalTreeT): void;
}

const TreeContext = createContext<TreeApi | null>(null);
const ruleKinds = ['USER', 'STARTER', 'LEADER', 'POSITION', 'ROLE', 'GROUP', 'EXPR'];
const timerActions: [string, string][] = [
  ['', '仅广播事件'],
  ['auto_approve', '自动通过'],
  ['auto_reject', '自动驳回'],
];

function metaOf(node: DesignerNode): string {
  const parts: string[] = [];
  const rules = node.approver?.rules ?? [];
  if (rules.length) {
    parts.push(rules.map((r) => (r.kind === 'USER' || r.kind === 'ROLE' ? `${r.kind}:${r.value}` : r.kind)).join(' / '));
  }
  if (node.approver?.strategy === 'ANY') parts.push('或签');
  if (node.approver?.sequential) parts.push('串行');
  if (node.approver?.collection) parts.push(`集合 ${node.approver.collection}`);
  if (node.timer?.duration) parts.push(`超时 ${node.timer.duration}`);
  return parts.join(' · ') || '未配置办理人';
}

function Connector({ onAdd, title }: { onAdd: () => void; title: string }) {
  return (
    <div className="gwf-connector">
      <button className="gwf-add" title={title} onClick={(e) => { e.stopPropagation(); onAdd(); }}>＋</button>
    </div>
  );
}

function NodeForm({
  node,
  onUpdate,
  onRules,
}: {
  node: DesignerNode;
  onUpdate: (patch: Partial<DesignerNode>) => void;
  onRules: (rules: ApproverRule[]) => void;
}) {
  const rules = node.approver?.rules ?? [];
  return (
    <div className="gwf-editor-inline" onClick={(e) => e.stopPropagation()}>
      <label>节点名称</label>
      <input value={node.name ?? ''} onChange={(e) => onUpdate({ name: e.target.value })} />
      <label>办理人规则</label>
      {rules.map((r, i) => (
        <div className="gwf-row" key={i}>
          <select value={r.kind} onChange={(e) => onRules(rules.map((x, j) => (j === i ? { ...x, kind: e.target.value as ApproverRule['kind'] } : x)))}>
            {ruleKinds.map((k) => (
              <option key={k} value={k}>{k}</option>
            ))}
          </select>
          <input value={r.value ?? ''} placeholder="编码 / 留空" onChange={(e) => onRules(rules.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))} />
          <button className="gwf-btn" style={{ color: 'var(--gwf-danger)' }} onClick={() => onRules(rules.filter((_, j) => j !== i))}>×</button>
        </div>
      ))}
      <button className="gwf-btn" onClick={() => onRules([...rules, { kind: 'USER', value: '' }])}>+ 规则</button>
      <label>审批策略</label>
      <select value={node.approver?.strategy ?? 'ALL'} onChange={(e) => onUpdate({ approver: { ...node.approver, strategy: e.target.value as 'ALL' | 'ANY' } })}>
        <option value="ALL">会签</option>
        <option value="ANY">或签</option>
      </select>
      <label>
        <input type="checkbox" checked={!!node.approver?.sequential} onChange={(e) => onUpdate({ approver: { ...node.approver, sequential: e.target.checked } })} />
        顺序多实例（逐个审批）
      </label>
      <label>集合表达式（多实例）</label>
      <input value={node.approver?.collection ?? ''} placeholder="如 items" onChange={(e) => onUpdate({ approver: { ...node.approver, collection: e.target.value } })} />
      <label>项变量名</label>
      <input value={node.approver?.elementVar ?? ''} placeholder="缺省 item" onChange={(e) => onUpdate({ approver: { ...node.approver, elementVar: e.target.value } })} />
      <label>任务超时</label>
      <input value={node.timer?.duration ?? ''} placeholder="如 24h" onChange={(e) => onUpdate({ timer: { ...node.timer, duration: e.target.value } })} />
      <label>超时动作</label>
      <select value={node.timer?.action ?? ''} onChange={(e) => onUpdate({ timer: { ...node.timer, action: e.target.value as '' | 'auto_approve' | 'auto_reject' } })}>
        {timerActions.map(([v, label]) => (
          <option key={v} value={v}>{label}</option>
        ))}
      </select>
    </div>
  );
}

function TaskCard({ node }: { node: DesignerNode }) {
  const api = useContext(TreeContext)!;
  const selected = api.selectedId === node.id;
  const [formOpen, setFormOpen] = useState(false);
  return (
    <div
      className={'gwf-v2-node' + (selected || formOpen ? ' selected' : '')}
      onClick={() => { api.select(node.id); setFormOpen((v) => !v); }}
    >
      <span className="gwf-v2-avatar">审</span>
      <div className="gwf-v2-name">{node.name || '审批节点'}</div>
      <div className="gwf-v2-meta">{metaOf(node)}</div>
      <div className="gwf-v2-actions">
        <button
          className="gwf-btn"
          style={{ color: 'var(--gwf-danger)' }}
          onClick={(e) => { e.stopPropagation(); api.commit(treeRemoveNode(api.tree, node.id)); }}
        >
          删除
        </button>
      </div>
      {formOpen ? (
        <NodeForm
          node={node}
          onUpdate={(patch) => api.commit(treeUpdateNode(api.tree, node.id, patch))}
          onRules={(rules) => api.commit(treeUpdateNode(api.tree, node.id, { approver: { ...node.approver, rules } }))}
        />
      ) : null}
    </div>
  );
}

function Level({ node }: { node: ApprovalTreeNode }) {
  const api = useContext(TreeContext)!;

  if (node.kind === 'task') {
    return (
      <>
        <TaskCard node={node.node} />
        <Connector onAdd={() => api.commit(treeAddTaskAfter(api.tree, node.node.id))} title="插入审批节点" />
        {node.child ? <Level node={node.child} /> : null}
      </>
    );
  }

  return (
    <div className="gwf-branch-cols">
      {node.branches.map((b) => (
        <div className="gwf-branch-col" key={b.id}>
          <div className="gwf-branch-tag">
            <span className="gwf-badge">{b.isDefault ? '默认' : '条件'}</span>
            {!b.isDefault ? (
              <input
                value={b.condition ?? ''}
                placeholder="条件表达式，如 amount > 1000"
                onChange={(e) => {
                  const t = structuredClone(api.tree);
                  for (const gate of allGates(t)) {
                    const br = gate.branches.find((x) => x.id === b.id);
                    if (br) {
                      br.condition = e.target.value;
                      break;
                    }
                  }
                  api.commit(t);
                }}
              />
            ) : null}
            <button className="gwf-btn" onClick={() => api.commit(treeAddBranch(api.tree, node.gateId))}>+ 分支</button>
          </div>
          {b.child ? <Level node={b.child} /> : (
            <div className="gwf-tree-add">
              <button className="gwf-btn" onClick={() => api.commit(treeAppendTaskToBranch(api.tree, b.id))}>+ 添加审批节点</button>
            </div>
          )}
        </div>
      ))}
    </div>
  );
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

  const rootRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    ensureStyles();
    if (rootRef.current) {
      if (theme) applyTheme(rootRef.current, theme);
      if (accent) applyAccent(rootRef.current, accent);
    }
  }, [theme, accent]);

  const commit = (t: ApprovalTreeT | null) => {
    setTree(t);
    if (t && onChange) onChange(treeToDocument(t));
  };

  const api: TreeApi = useMemo(() => ({
    tree: (tree ?? { key: doc.key, root: null, end: { id: 'end' } }) as ApprovalTreeT,
    selectedId,
    select(id: string) { setSelectedId(id); },
    commit(t2: ApprovalTreeT | null) { commit(t2); },
  }), [tree, selectedId, onChange]);

  useImperativeHandle(ref, () => ({
    getDocument() {
      return tree ? treeToDocument(tree) : null;
    },
  }));

  return (
    <div
      ref={rootRef}
      className="gwf-designer"
      data-gwf-theme={theme ?? 'auto'}
      style={{ ...(accent ? { ['--gwf-accent' as string]: accent } : {}), height: height ?? 'auto', overflowY: 'auto' }}
    >
      {tree === null ? (
        <div style={{ padding: 12, color: 'var(--gwf-danger)' }}>不适用审批树模式（请使用 BPMN 画布）</div>
      ) : (
        <TreeContext.Provider value={api}>
          <div className="gwf-tree-area">
            <div className="gwf-start-node">发起申请</div>
            <Connector onAdd={() => api.commit(treeAddTaskFirst(tree))} title="插入审批节点" />
            {tree.root ? <Level node={tree.root} /> : null}
            <div className="gwf-end-node">结束</div>
          </div>
        </TreeContext.Provider>
      )}
    </div>
  );
});
