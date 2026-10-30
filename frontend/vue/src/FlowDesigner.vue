<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { createDesigner, type DesignerDocument, type DesignerHandle, type Theme } from '@go-wind/flow-designer-core';

const props = defineProps<{
  /** BPMN XML 或 designer-json-v1 文档（自动嗅探：'<' 开头为 XML）。 */
  xml?: string;
  document?: DesignerDocument;
  /** 画布高度（CSS 值，缺省 600px）。 */
  height?: string;
  /** 主题：light / dark / auto（缺省跟随系统）。 */
  theme?: Theme;
  /** 自定义主题色（--gwf-accent）。 */
  accent?: string;
}>();

const emit = defineEmits<{
  (e: 'ready', handle: DesignerHandle): void;
  (e: 'error', err: unknown): void;
}>();

const canvasRef = ref<HTMLDivElement>();
const panelRef = ref<HTMLDivElement>();
let handle: DesignerHandle | null = null;

onMounted(async () => {
  if (!canvasRef.value || !panelRef.value) return;
  handle = createDesigner({
    canvas: canvasRef.value,
    panel: panelRef.value,
    theme: props.theme ?? 'auto',
    accent: props.accent,
  });
  emit('ready', handle);
  try {
    if (props.xml) {
      await handle.loadXML(props.xml);
    } else if (props.document) {
      await handle.loadDocument(props.document);
    } else {
      await handle.loadXML(emptyXML());
    }
    handle.zoom('fit-viewport');
  } catch (err) {
    emit('error', err);
  }
});

watch(
  () => props.theme,
  (t) => handle?.setTheme(t ?? 'auto'),
);
watch(
  () => props.accent,
  (c) => { if (c) handle?.setAccent(c); },
);
watch(
  () => props.xml,
  async (xml) => {
    if (xml && handle) {
      try {
        await handle.loadXML(xml);
      } catch (err) {
        emit('error', err);
      }
    }
  },
);

onBeforeUnmount(() => handle?.destroy());

defineExpose({
  /** 导出 BPMN XML（保存 → BFF → Deploy）。 */
  async getXML(): Promise<string> {
    return handle ? handle.getXML() : '';
  },
  /** 导出 designer-json-v1 文档。 */
  async getDocument(key?: string): Promise<DesignerDocument> {
    if (!handle) throw new Error('designer not mounted');
    return handle.getDocument(key);
  },
});

function emptyXML(): string {
  return `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" targetNamespace="https://github.com/tx7do/go-wind-flow">
  <process id="new_process" isExecutable="true">
    <startEvent id="Start_1"/>
    <userTask id="Task_1"/>
    <endEvent id="End_1"/>
    <sequenceFlow id="F_1" sourceRef="Start_1" targetRef="Task_1"/>
    <sequenceFlow id="F_2" sourceRef="Task_1" targetRef="End_1"/>
  </process>
</definitions>`;
}
</script>

<template>
  <div class="gwf-designer">
    <div ref="canvasRef" class="gwf-canvas" :style="{ height: height ?? '600px' }" />
    <div ref="panelRef" class="gwf-panel" />
  </div>
</template>

<style scoped>
.gwf-designer {
  display: flex;
  gap: 8px;
}
.gwf-canvas {
  flex: 1;
  border: 1px solid var(--gwf-border);
}
.gwf-panel {
  width: 320px;
  border: 1px solid var(--gwf-border);
  overflow-y: auto;
}
</style>
