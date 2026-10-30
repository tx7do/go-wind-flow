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
  formOpenId: { value: string };
  toggleForm: (id: string) => void;
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
}>(Symbol.for("gwf-tree-api"))!;

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
  const formOpen = selected && api.formOpenId.value === node.id;
  const children: any[] = [
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
  ];
  if (formOpen) {
    const rules = node.approver?.rules ?? [];
    children.push(
      h('div', { class: 'gwf-editor-inline', onClick: (e: Event) => e.stopPropagation() }, [
        h('label', '审批策略'),
        h('select', {
          value: node.approver?.strategy ?? 'ALL',
          onChange: (e: Event) => api.update(node.id, { approver: { ...node.approver, strategy: (e.target as HTMLSelectElement).value } }),
        }, [h('option', { value: 'ALL' }, '会签'), h('option', { value: 'ANY' }, '或签')]),
        h('label', '顺序多实例（逐个审批）'),
        h('input', {
          type: 'checkbox',
          checked: !!node.approver?.sequential,
          onChange: (e: Event) => api.update(node.id, { approver: { ...node.approver, sequential: (e.target as HTMLInputElement).checked } }),
        }),
        h('label', '集合表达式（多实例）'),
        h('input', {
          value: node.approver?.collection ?? '',
          placeholder: '如 items',
          onInput: (e: Event) => api.update(node.id, { approver: { ...node.approver, collection: (e.target as HTMLInputElement).value } }),
        }),
        h('label', '项变量名'),
        h('input', {
          value: node.approver?.elementVar ?? '',
          placeholder: '缺省 item',
          onInput: (e: Event) => api.update(node.id, { approver: { ...node.approver, elementVar: (e.target as HTMLInputElement).value } }),
        }),
        h('label', '任务超时'),
        h('input', {
          value: node.timer?.duration ?? '',
          placeholder: '如 24h',
          onInput: (e: Event) => api.update(node.id, { timer: { ...node.timer, duration: (e.target as HTMLInputElement).value } }),
        }),
        h('label', '超时动作'),
        h('select', {
          value: node.timer?.action ?? '',
          onChange: (e: Event) => api.update(node.id, { timer: { ...node.timer, action: (e.target as HTMLSelectElement).value } }),
        }, [
          h('option', { value: '' }, '仅广播事件'),
          h('option', { value: 'auto_approve' }, '自动通过'),
          h('option', { value: 'auto_reject' }, '自动驳回'),
        ]),
      ]),
    );
  }
  return h(
    'div',
    {
      class: ['gwf-v2-node', { selected }],
      onClick: () => {
        api.select(node.id);
        api.toggleForm(node.id);
      },
    },
    children,
  );
}
</script>

<template>
  <template v-if="node.kind === 'task'">
    <template v-for="n in [node]" :key="n.node.id">
      <component :is="taskCard(n.node, true)" />
    </template>
    <div class="gwf-connector">
      <button class="gwf-add" title="插入审批节点" @click.stop="api.addAfter(node.node.id)">+</button>
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
      <button class="gwf-add" title="插入审批节点" @click.stop="api.addAfterBranches(node.gateId)">+</button>
    </div>
    <TreeLevel v-if="node.child" :node="node.child" />
  </template>
</template>
