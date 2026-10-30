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
