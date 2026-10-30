<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  documentToTree,
  treeAddBranch,
  treeAddTaskAfter,
  treeRemoveNode,
  treeToDocument,
  treeUpdateNode,
  UnsupportedError,
  type ApprovalTree,
  type ApprovalTreeBranch,
  type ApprovalTreeNode,
  type DesignerDocument,
  type DesignerNode,
  type ApproverRule,
  type Theme,
} from '@go-wind/flow-designer-core';

const props = defineProps<{
  document: DesignerDocument;
  theme?: Theme;
  accent?: string;
}>();

const emit = defineEmits<{
  (e: 'change', doc: DesignerDocument): void;
  (e: 'unsupported', reason: string): void;
}>();

const selectedId = ref('');
const unsupported = ref('');
let tree: ApprovalTree | null = null;
const version = ref(0);

function init() {
  unsupported.value = '';
  try {
    tree = documentToTree(props.document);
  } catch (err) {
    if (err instanceof UnsupportedError) {
      unsupported.value = err.message;
      tree = null;
    } else {
      throw err;
    }
  }
}
init();

function commit() {
  version.value++;
  if (tree) emit('change', treeToDocument(tree));
}

interface Row {
  key: string;
  depth: number;
  kind: 'node' | 'branch-head';
  node?: DesignerNode;
  branch?: ApprovalTreeBranch;
  gateId?: string;
}

const rows = computed<Row[]>(() => {
  void version.value;
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
      out.push({ key: cur.gateId + ':head', depth, kind: 'branch-head' });
      for (const b of cur.branches) {
        depth++;
        out.push({ key: b.id + ':head', depth, kind: 'branch-head', branch: b, gateId: cur.gateId });
        walk(b.child);
        depth--;
      }
      cur = cur.child;
    }
    depth = Math.max(0, depth - 1);
  };
  walk(tree?.root ?? null);
  return out;
});

const selectedNode = computed<DesignerNode | null>(() => findNode(selectedId.value));

function findNode(id: string): DesignerNode | null {
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
}
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

function onSelect(id: string) {
  selectedId.value = selectedId.value === id ? '' : id;
}
function onAddAfter(id: string) {
  if (!tree) return;
  tree = treeAddTaskAfter(tree, id);
  commit();
}
function onRemove(id: string) {
  if (!tree) return;
  tree = treeRemoveNode(tree, id);
  selectedId.value = '';
  commit();
}
function onAddBranch(gateId: string) {
  if (!tree) return;
  tree = treeAddBranch(tree, gateId);
  commit();
}
function onUpdate(id: string, patch: Partial<DesignerNode>) {
  if (!tree) return;
  tree = treeUpdateNode(tree, id, patch);
  commit();
}
function onBranchUpdate(branchId: string, patch: Partial<ApprovalTreeBranch>) {
  if (!tree) return;
  const doc = treeToDocument(tree);
  const edge = doc.edges.find((e) => e.id === branchId);
  if (!edge) return;
  if ('name' in patch) edge.name = patch.name;
  if ('condition' in patch) edge.condition = patch.condition;
  if (patch.isDefault) {
    doc.edges.forEach((e) => {
      if (e.source === edge.source && e.id !== branchId) e.isDefault = undefined;
    });
    edge.isDefault = true;
    const gw = doc.nodes.find((n) => n.id === edge.source);
    if (gw) gw.defaultEdge = branchId;
  }
  try {
    tree = documentToTree(doc);
  } catch (err) {
    if (err instanceof UnsupportedError) unsupported.value = err.message;
  }
  commit();
}

function nodeRules(id: string): ApproverRule[] {
  return findNode(id)?.approver?.rules ?? [];
}
function setRules(id: string, rules: ApproverRule[]) {
  const n = findNode(id);
  onUpdate(id, { approver: { ...(n?.approver ?? {}), rules } });
}
const ruleKinds = ['USER', 'STARTER', 'LEADER', 'POSITION', 'ROLE', 'GROUP', 'EXPR'];

function metaOf(node: DesignerNode): string {
  const parts: string[] = [];
  const rules = node.approver?.rules ?? [];
  if (rules.length) {
    parts.push(rules.map((r) => (r.kind === 'USER' || r.kind === 'ROLE' ? `${r.kind}:${r.value}` : r.kind)).join(' / '));
  }
  if (node.approver?.strategy === 'ANY') parts.push('或签');
  if (node.approver?.sequential) parts.push('串行');
  if (node.timer?.duration) parts.push(`超时 ${node.timer.duration}`);
  return parts.join(' · ') || '未配置办理人';
}

const themeAttrs = computed(() => ({
  'data-gwf-theme': props.theme ?? 'auto',
  style: props.accent ? { '--gwf-accent': props.accent } : undefined,
}));
</script>

<template>
  <div ref="rootEl" class="gwf-designer gwf-tree" v-bind="themeAttrs">
    <div v-if="unsupported" class="gwf-unsupported">不适用审批树模式：{{ unsupported }}（请使用 BPMN 画布）</div>
    <template v-else>
      <div class="gwf-tree-node">
        <div class="gwf-node-name">发起申请</div>
      </div>
      <div class="gwf-tree-connector" />
      <template v-for="row in rows" :key="row.key">
        <div v-if="row.kind === 'node' && row.node" class="gwf-tree-node"
          :class="{ selected: selectedId === row.node.id }">
          <div class="gwf-node-name">{{ row.node.name || '审批节点' }}</div>
          <div class="gwf-node-meta">{{ metaOf(row.node) }}</div>
          <div class="gwf-node-actions">
            <button class="gwf-btn" @click.stop="onAddAfter(row.node!.id)">+ 节点</button>
            <button class="gwf-btn danger" @click.stop="onRemove(row.node!.id)">删除</button>
          </div>
          <div v-if="selectedId === row.node!.id" class="gwf-form">
            <label>节点名称
              <input :value="row.node!.name" @input="onUpdate(row.node!.id, { name: ($event.target as HTMLInputElement).value })" />
            </label>
            <label>办理人规则</label>
            <div v-for="(r, i) in nodeRules(row.node!.id)" :key="i" class="gwf-row">
              <select :value="r.kind" @change="setRules(row.node!.id, nodeRules(row.node!.id).map((x, j) => j === i ? { ...x, kind: ($event.target as HTMLSelectElement).value as ApproverRule['kind'] } : x))">
                <option v-for="k in ruleKinds" :key="k" :value="k">{{ k }}</option>
              </select>
              <input :value="r.value" placeholder="编码 / 留空"
                @input="setRules(row.node!.id, nodeRules(row.node!.id).map((x, j) => j === i ? { ...x, value: ($event.target as HTMLInputElement).value } : x))" />
              <button class="gwf-btn danger"
                @click="setRules(row.node!.id, nodeRules(row.node!.id).filter((_, j) => j !== i))">×</button>
            </div>
            <button class="gwf-btn" @click="setRules(row.node!.id, [...nodeRules(row.node!.id), { kind: 'USER', value: '' }])">+
              规则</button>
            <label>审批策略
              <select :value="row.node!.approver?.strategy ?? 'ALL'"
                @change="onUpdate(row.node!.id, { approver: { ...row.node!.approver, strategy: ($event.target as HTMLSelectElement).value as 'ALL' | 'ANY' } })">
                <option value="ALL">会签</option>
                <option value="ANY">或签</option>
              </select>
            </label>
            <label>任务超时
              <input :value="row.node!.timer?.duration ?? ''" placeholder="如 24h"
                @input="onUpdate(row.node!.id, { timer: { ...row.node!.timer, duration: ($event.target as HTMLInputElement).value } })" />
            </label>
            <label>超时动作
              <select :value="row.node!.timer?.action ?? ''"
                @change="onUpdate(row.node!.id, { timer: { ...row.node!.timer, action: ($event.target as HTMLSelectElement).value as '' | 'auto_approve' | 'auto_reject' } })">
                <option value="">仅广播事件</option>
                <option value="auto_approve">自动通过</option>
                <option value="auto_reject">自动驳回</option>
              </select>
            </label>
          </div>
        </div>
        <div v-if="row.kind === 'node'" class="gwf-tree-connector" />
        <div v-if="row.kind === 'branch-head' && row.branch" class="gwf-tree-branch" :style="{ marginLeft: row.depth * 8 + 'px' }">
          <div class="gwf-branch-head">
            <span class="gwf-badge">{{ row.branch.isDefault ? '默认' : '条件' }}</span>
            <input v-if="!row.branch.isDefault" :value="row.branch.condition ?? ''" placeholder="条件表达式，如 amount > 1000"
              @change="onBranchUpdate(row.branch!.id, { condition: ($event.target as HTMLInputElement).value })" />
            <input v-else :value="row.branch.name" readonly />
            <button class="gwf-btn" @click="onAddBranch(row.gateId!)">+ 分支</button>
          </div>
        </div>
      </template>
      <div class="gwf-tree-add">
        <button class="gwf-btn" @click="onAddAfter(rows.length ? (rows[rows.length - 1].node?.id ?? '') : '')">+
          审批节点</button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.gwf-unsupported {
  padding: 12px;
  color: var(--gwf-danger);
}
</style>
