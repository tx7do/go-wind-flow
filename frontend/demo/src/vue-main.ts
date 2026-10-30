import { createApp, h, onMounted, ref } from 'vue';
import {
  createDesigner,
  ensureStyles,
  type Theme,
} from '@go-wind/flow-designer-core';
import { ApprovalTree as ApprovalTreeVue } from '@go-wind/flow-designer-vue';
import { injectDemoStyles, leaveApprovalXML, renderToolbar, treeSampleDoc } from './shared';

import 'bpmn-js/dist/assets/diagram-js.css';
import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';
import 'bpmn-js/dist/assets/bpmn-js.css';
import '@bpmn-io/properties-panel/dist/assets/properties-panel.css';

ensureStyles();
injectDemoStyles();

const app = createApp({
  setup() {
    const theme = ref<Theme>('auto');
    const accent = ref('#00add8');
    const tab = ref<'canvas' | 'tree'>('canvas');
    const output = ref('');
    let treeDoc = structuredClone(treeSampleDoc);
    let designer: ReturnType<typeof createDesigner> | null = null;

    // 画布容器 ref：保留元素，等整个子树挂载完再创建（同级面板此刻才在 DOM 里）。
    let canvasEl: HTMLElement | null = null;
    const canvasRef = (el: unknown) => {
      if (el) canvasEl = el as HTMLElement;
    };
    onMounted(() => {
      if (!canvasEl || designer) return;
      const wrap = canvasEl.parentElement!;
      const panel = wrap.querySelector('.gwf-panel') as HTMLElement | null;
      designer = createDesigner({ canvas: canvasEl, panel: panel ?? undefined, theme: theme.value, accent: accent.value });
      void designer.loadXML(leaveApprovalXML).then(() => designer?.zoom('fit-viewport'));
    });

    const setTheme = (t: Theme) => {
      theme.value = t;
      designer?.setTheme(t);
    };
    const setAccent = (c: string) => {
      accent.value = c;
      designer?.setAccent(c);
    };
    const save = () => {
      if (tab.value === 'canvas') {
        void designer?.getXML().then((xml) => (output.value = xml ?? ''));
      } else if (treeDoc) {
        output.value = JSON.stringify(treeDoc, null, 2);
      }
    };

    return () =>
      h('div', { class: 'demo-page' }, [
        h('div', { class: 'demo-toolbar' }, [
          h('span', { class: 'demo-brand' }, 'go-wind-flow 设计器 Demo（Vue3）'),
          h(
            'span',
            { class: 'demo-group' },
            ([['light', '亮色'], ['dark', '暗色'], ['auto', '跟随系统']] as [Theme, string][]).map(([t, label]) =>
              h(
                'button',
                {
                  key: t,
                  style: theme.value === t ? { background: '#00add8', color: '#fff', borderColor: '#00add8' } : {},
                  onClick: () => setTheme(t),
                },
                label,
              ),
            ),
          ),
          h('label', { class: 'demo-accent' }, [
            '主题色',
            h('input', {
              type: 'color',
              value: accent.value,
              onInput: (ev: Event) => setAccent((ev.target as HTMLInputElement).value),
            }),
          ]),
          h('button', { class: 'demo-save', onClick: save }, '保存'),
        ]),
        h('div', { class: 'demo-tabs' }, [
          h(
            'button',
            { class: tab.value === 'canvas' ? 'active' : '', onClick: () => (tab.value = 'canvas') },
            'BPMN 画布（专业模式）',
          ),
          h(
            'button',
            { class: tab.value === 'tree' ? 'active' : '', onClick: () => (tab.value = 'tree') },
            '审批树（业务模式）',
          ),
        ]),
        // 两个视图都保持挂载（display 切换），bpmn-js 实例不随 Tab 卸载。
        h('div', { class: 'demo-canvas-wrap', style: { display: tab.value === 'canvas' ? 'flex' : 'none' } }, [
          h('div', { class: 'gwf-canvas', ref: canvasRef }),
          h('div', { class: 'gwf-panel' }),
        ]),
        h('div', { style: { display: tab.value === 'canvas' ? 'none' : 'block' } }, [
          h(ApprovalTreeVue, {
            document: treeDoc,
            theme: theme.value,
            accent: accent.value,
            onChange: (d: DesignerDocument) => {
              treeDoc = d;
            },
          }),
        ]),
        h('textarea', {
          class: 'demo-output',
          readonly: true,
          value: output.value,
          placeholder: '点「保存」查看导出结果（BPMN XML / designer JSON）',
        }),
      ]);
  },
});

app.mount('#app');
