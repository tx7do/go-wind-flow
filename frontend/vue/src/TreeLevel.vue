<script setup lang="ts">
/**
 * 递归层级：渲染任务链与分支构造（列）。
 * 操作集经 inject('gwf-tree-api') 注入；选中节点的编辑在宿主侧边编辑器完成。
 */
import { h, inject } from 'vue';
import type { ApprovalTreeNode } from '@go-wind/flow-designer-core';

defineProps<{ node: ApprovalTreeNode }>();

const api = inject<{
  selectedId: { value: string };
  select: (id: string) => void;
  addAfter: (id: string) => void;
  addFirst: () => void;
  appendBranch: (branchId: string) => void;
  addBranch: (gateId: string) => void;
  addAfterBranches: (gateId: string) => void;
  remove: (id: string) => void;
  removeBranch: (gateId: string, branchId: string) => void;
  update: (id: string, patch: Record<string, unknown>) => void;
  updateBranch: (branchId: string, patch: Record<string, unknown>) => void;
  nodeOf: (id: string) => Record<string, any> | null;
}>(Symbol.for('gwf-tree-api'))!;

function metaOf(node: any): string {
  const parts: string[] = [];
  const rules = node.approver?.rules ?? [];
  if (rules.length) {
    parts.push(
      rules.map((r: any) => (r.kind === 'USER' || r.kind === 'ROLE' ? `${r.kind}:${r.value}` : r.kind)).join(' / '),
    );
  }
  if (node.approver?.strategy === 'ANY') parts.push('或签');
  if (node.approver?.sequential) parts.push('串行');
  if (node.approver?.collection) parts.push(`集合 ${node.approver.collection}`);
  if (node.timer?.duration) parts.push(`超时 ${node.timer.duration}`);
  return parts.join(' · ') || '未配置办理人';
}

function taskCard(node: any, removable: boolean) {
  const selected = api.selectedId.value === node.id;
  return h(
    'div',
    { class: ['gwf-v2-node', { selected }], onClick: () => api.select(node.id) },
    [
      h('span', { class: 'gwf-v2-avatar' }, '审'),
      h('div', { class: 'gwf-v2-name' }, node.name || '审批节点'),
      h('div', { class: 'gwf-v2-meta' }, metaOf(node)),
      removable
        ? h('div', { class: 'gwf-v2-actions' }, [
            h('button', {
              class: 'gwf-btn danger',
              onClick: (e: Event) => { e.stopPropagation(); api.remove(node.id); },
            }, '删除'),
          ])
        : null,
    ],
  );
}
</script>

<template>
  <template v-if="node.kind === 'task'">
    <template v-for="n in [node]" :key="n.node.id">
      <component :is="taskCard(n.node, true)" />
    </template>
    <div class="gwf-connector">
      <button class="gwf-add" title="插入审批节点" @click.stop="api.addAfter(node.node.id)">＋</button>
    </div>
    <TreeLevel v-if="node.child" :node="node.child" />
  </template>

  <template v-else>
    <div class="gwf-branch-cols">
      <div v-for="b in node.branches" :key="b.id" class="gwf-branch-col">
        <div class="gwf-branch-tag">
          <span class="gwf-badge">{{ b.isDefault ? '默认' : '条件' }}</span>
          <input
            v-if="!b.isDefault"
            :value="b.condition ?? ''"
            placeholder="条件表达式"
            @change="api.updateBranch(b.id, { condition: ($event.target as HTMLInputElement).value })"
          />
          <button
            v-if="node.branches.length > 1"
            class="gwf-btn danger"
            title="删除分支"
            @click.stop="api.removeBranch(node.gateId, b.id)"
          >✕</button>
        </div>
        <div v-if="b.child && b.child.kind === 'task'" :key="'c-' + b.child.node.id">
          <component :is="taskCard(b.child.node, true)" />
        </div>
        <div v-else-if="!b.child" class="gwf-tree-add">
          <button class="gwf-btn" @click.stop="api.appendBranch(b.id)">+ 添加审批节点</button>
        </div>
        <TreeLevel v-if="b.child && b.child.child" :node="b.child.child" />
      </div>
      <div class="gwf-branch-tag" style="justify-content: center">
        <button class="gwf-btn" @click.stop="api.addBranch(node.gateId)">+ 添加分支</button>
      </div>
    </div>
    <div class="gwf-connector">
      <button class="gwf-add" title="插入审批节点" @click.stop="api.addAfterBranches(node.gateId)">＋</button>
    </div>
    <TreeLevel v-if="node.child" :node="node.child" />
  </template>
</template>
