<script setup lang="ts">
/**
 * 垂直审批树（钉钉式业务模式，Vue3）。
 * 左侧树渲染（TreeLevel 递归）+ 右侧选中节点编辑器。
 * 保存导出 designer-json-v1；子集外文档抛 UnsupportedError 时回退画布。
 */
import { computed, provide, ref, shallowRef, watch } from "vue";
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
  ensureStyles,
  type ApprovalTree,
  type ApprovalTreeNode,
  type DesignerDocument,
  type DesignerNode,
  type Theme,
} from '@go-wind/flow-designer-core';
import TreeLevel from './TreeLevel.vue';

const props = defineProps<{
  document: DesignerDocument;
  theme?: Theme;
  accent?: string;
}>();

const emit = defineEmits<{
  (e: 'change', doc: DesignerDocument): void;
  (e: 'unsupported', reason: string): void;
}>();

ensureStyles();

const unsupported = ref('');
const tree = shallowRef<ApprovalTree | null>(null);
try {
  tree.value = documentToTree(props.document);
} catch (err) {
  if (err instanceof UnsupportedError) {
    unsupported.value = err.message;
    tree.value = null;
  } else {
    throw err;
  }
}

const selectedId = ref('');
const formOpenId = ref('');

const api = {
  selectedId,
  formOpenId,
  select(id: string) {
    selectedId.value = selectedId.value === id ? '' : id;
  },
  toggleForm(id: string) {
    formOpenId.value = formOpenId.value === id ? '' : id;
  },
  addAfter(id: string) {
    if (tree.value) commit(treeAddTaskAfter(tree.value, id));
  },
  addFirst() {
    if (tree.value) commit(treeAddTaskFirst(tree.value));
  },
  appendBranch(branchId: string) {
    if (tree.value) commit(treeAppendTaskToBranch(tree.value, branchId));
  },
  addBranch(gateId: string) {
    if (tree.value) commit(treeAddBranch(tree.value, gateId));
  },
  addAfterBranches(gateId: string) {
    if (tree.value) commit(treeAddTaskAfterBranches(tree.value, gateId));
  },
  remove(id: string) {
    if (tree.value) commit(treeRemoveNode(tree.value, id));
    if (selectedId.value === id) selectedId.value = '';
  },
  removeBranch(gateId: string, branchId: string) {
    if (tree.value) commit(treeRemoveBranch(tree.value, gateId, branchId));
  },
  update(id: string, patch: Record<string, unknown>) {
    if (tree.value) commit(treeUpdateNode(tree.value, id, patch));
  },
  updateBranch(branchId: string, patch: Record<string, unknown>) {
    if (!tree.value) return;
    const d = treeToDocument(tree.value);
    const edge = d.edges.find((e) => e.id === branchId);
    if (!edge) return;
    if ('name' in patch) edge.name = patch.name as string | undefined;
    if ('condition' in patch) edge.condition = patch.condition as string | undefined;
    if (patch.isDefault) {
      d.edges.forEach((e) => {
        if (e.source === edge.source && e.id !== branchId) e.isDefault = undefined;
      });
      edge.isDefault = true;
      const gw = d.nodes.find((n) => n.id === edge.source);
      if (gw) gw.defaultEdge = branchId;
    }
    try {
      tree.value = documentToTree(d);
      commit(tree.value);
    } catch (err) {
      if (err instanceof UnsupportedError) unsupported.value = err.message;
    }
  },
  nodeOf(id: string): DesignerNode | null {
    return findNode(id);
  },
};
provide(Symbol.for('gwf-tree-api'), api);

function commit(t: ApprovalTree) {
  tree.value = t;
  emit("change", treeToDocument(t));
}

function findNode(id: string): DesignerNode | null {
  let cur: ApprovalTreeNode | null = tree.value?.root ?? null;
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
function findDeep(node: ApprovalTreeNode | null, id: string): DesignerNode | null {
  let cur: ApprovalTreeNode | null = node;
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

const selected = computed<DesignerNode | null>(() => (selectedId.value ? findNode(selectedId.value) : null));

const themeAttrs = computed(() => ({
  'data-gwf-theme': props.theme ?? 'auto',
  style: props.accent ? { '--gwf-accent': props.accent } : undefined,
}));
</script>

<template>
  <div class="gwf-designer" v-bind="themeAttrs">
    <div v-if="unsupported" class="gwf-unsupported">不适用审批树模式：{{ unsupported }}（请使用 BPMN 画布）</div>
    <div v-else class="gwf-tree-wrap">
      <div class="gwf-tree-area">
        <div class="gwf-start-node">发起申请</div>
        <div class="gwf-connector">
          <button class="gwf-add" title="插入审批节点" @click.stop="api.addFirst()">＋</button>
        </div>
        <TreeLevel v-if="tree?.root" :node="tree.root" />
        <div class="gwf-connector"><div class="gwf-line" /></div>
        <div class="gwf-end-node">结束</div>
      </div>
      <div v-if="selected" class="gwf-editor">
        <h4>节点配置</h4>
        <label>节点名称</label>
        <input :value="selected.name" @input="api.update(selected.id, { name: ($event.target as HTMLInputElement).value })" />
        <label>办理人规则</label>
        <div v-for="(r, i) in selected.approver?.rules ?? []" :key="i" class="gwf-row">
          <select :value="r.kind" @change="api.update(selected.id, { approver: { ...selected.approver, rules: (selected.approver?.rules ?? []).map((x, j) => j === i ? { ...x, kind: ($event.target as HTMLSelectElement).value } : x) } })">
            <option value="USER">USER</option>
            <option value="STARTER">STARTER</option>
            <option value="LEADER">LEADER</option>
            <option value="POSITION">POSITION</option>
            <option value="ROLE">ROLE</option>
            <option value="GROUP">GROUP</option>
            <option value="EXPR">EXPR</option>
          </select>
          <input :value="r.value" placeholder="编码 / 留空" @input="api.update(selected.id, { approver: { ...selected.approver, rules: (selected.approver?.rules ?? []).map((x, j) => j === i ? { ...x, value: ($event.target as HTMLInputElement).value } : x) } })" />
          <button class="gwf-btn danger" @click="api.update(selected.id, { approver: { ...selected.approver, rules: (selected.approver?.rules ?? []).filter((_, j) => j !== i) } })">×</button>
        </div>
        <button class="gwf-btn" @click="api.update(selected.id, { approver: { ...selected.approver, rules: [...(selected.approver?.rules ?? []), { kind: 'USER', value: '' }] } })">+ 规则</button>
        <label>审批策略</label>
        <select :value="selected.approver?.strategy ?? 'ALL'" @change="api.update(selected.id, { approver: { ...selected.approver, strategy: ($event.target as HTMLSelectElement).value } })">
          <option value="ALL">会签</option>
          <option value="ANY">或签</option>
        </select>
        <label>
          <input type="checkbox" :checked="selected.approver?.sequential" @change="api.update(selected.id, { approver: { ...selected.approver, sequential: ($event.target as HTMLInputElement).checked } })" />
          顺序多实例（逐个审批）
        </label>
        <label>集合表达式（多实例）</label>
        <input :value="selected.approver?.collection ?? ''" placeholder="如 items" @input="api.update(selected.id, { approver: { ...selected.approver, collection: ($event.target as HTMLInputElement).value } })" />
        <label>项变量名</label>
        <input :value="selected.approver?.elementVar ?? ''" placeholder="缺省 item" @input="api.update(selected.id, { approver: { ...selected.approver, elementVar: ($event.target as HTMLInputElement).value } })" />
        <label>任务超时</label>
        <input :value="selected.timer?.duration ?? ''" placeholder="如 24h" @input="api.update(selected.id, { timer: { ...selected.timer, duration: ($event.target as HTMLInputElement).value } })" />
        <label>超时动作</label>
        <select :value="selected.timer?.action ?? ''" @change="api.update(selected.id, { timer: { ...selected.timer, action: ($event.target as HTMLSelectElement).value } })">
          <option value="">仅广播事件</option>
          <option value="auto_approve">自动通过</option>
          <option value="auto_reject">自动驳回</option>
        </select>
      </div>
    </div>
  </div>
</template>

<style scoped>
.gwf-unsupported {
  padding: 12px;
  color: var(--gwf-danger);
}
.gwf-tree-wrap {
  display: flex;
}
.gwf-tree-area {
  flex: 1;
}
</style>
