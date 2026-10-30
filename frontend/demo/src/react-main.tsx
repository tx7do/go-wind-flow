import { createRoot } from 'react-dom/client';
import { createElement, useEffect, useRef, useState } from 'react';
import {
  createDesigner,
  ensureStyles,
  type Theme,
} from '@go-wind/flow-designer-core';
import { ApprovalTree, FlowDesigner } from '@go-wind/flow-designer-react';
import { injectDemoStyles, leaveApprovalXML, renderToolbar, treeSampleDoc, type ThemeMode } from './shared';

import 'bpmn-js/dist/assets/diagram-js.css';
import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';
import 'bpmn-js/dist/assets/bpmn-js.css';
import '@bpmn-io/properties-panel/dist/assets/properties-panel.css';

ensureStyles();
injectDemoStyles();

function App() {
  const [theme, setThemeState] = useState<Theme>('auto');
  const [accent, setAccentState] = useState('#00add8');
  const [tab, setTab] = useState<'canvas' | 'tree'>('canvas');
  const [output, setOutput] = useState('');
  const [treeDoc, setTreeDoc] = useState(structuredClone(treeSampleDoc));
  const canvasRef = useRef<HTMLDivElement>(null);
  const designerRef = useRef<ReturnType<typeof createDesigner> | null>(null);

  const setTheme = (t: Theme) => {
    setThemeState(t);
    designerRef.current?.setTheme(t);
  };
  const setAccent = (c: string) => {
    setAccentState(c);
    designerRef.current?.setAccent(c);
  };
  const save = () => {
    if (tab === 'canvas') {
      void designerRef.current?.getXML().then((xml) => setOutput(xml ?? ''));
    } else {
      setOutput(JSON.stringify(treeDoc, null, 2));
    }
  };

  const onCanvasMount = (el: HTMLDivElement | null) => {
    canvasRef.current = el;
  };
  // 等整棵子树挂载完再建（ref 回调触发时同级面板尚未插入 DOM）。
  useEffect(() => {
    const el = canvasRef.current;
    if (!el || designerRef.current) return;
    const wrap = el.parentElement!;
    const panel = wrap.querySelector('.gwf-panel') as HTMLElement | null;
    designerRef.current = createDesigner({ canvas: el, panel: panel ?? undefined, theme, accent });
    void designerRef.current.loadXML(leaveApprovalXML).then(() => designerRef.current?.zoom('fit-viewport'));
    // 仅在首次挂载时创建；主题/主题色另有 effect 同步。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return createElement(
    'div',
    { className: 'demo-page' },
    createElement(
      'div',
      { className: 'demo-toolbar' },
      createElement('span', { className: 'demo-brand' }, 'go-wind-flow 设计器 Demo（React）'),
      createElement(
        'span',
        { className: 'demo-group' },
        ([['light', '亮色'], ['dark', '暗色'], ['auto', '跟随系统']] as [Theme, string][]).map(([t, label]) =>
          createElement(
            'button',
            {
              key: t,
              style: theme === t ? { background: '#00add8', color: '#fff', borderColor: '#00add8' } : {},
              onClick: () => setTheme(t),
            },
            label,
          ),
        ),
      ),
      createElement('label', { className: 'demo-accent' }, [
        '主题色',
        createElement('input', { type: 'color', value: accent, onInput: (e: any) => setAccent(e.target.value) }),
      ]),
      createElement('button', { className: 'demo-save', onClick: save }, '保存'),
    ),
    createElement(
      'div',
      { className: 'demo-tabs' },
      createElement(
        'button',
        { className: tab === 'canvas' ? 'active' : '', onClick: () => setTab('canvas') },
        'BPMN 画布（专业模式）',
      ),
      createElement(
        'button',
        { className: tab === 'tree' ? 'active' : '', onClick: () => setTab('tree') },
        '审批树（业务模式）',
      ),
    ),
    // 两个视图都保持挂载（display 切换），bpmn-js 实例不随 Tab 卸载。
    createElement(
      'div',
      { className: 'demo-canvas-wrap', style: { display: tab === 'canvas' ? 'flex' : 'none' } },
      createElement('div', { className: 'gwf-canvas', ref: onCanvasMount }),
      createElement('div', { className: 'gwf-panel' }),
    ),
    createElement('div', { style: { display: tab === 'canvas' ? 'none' : 'block' } },
      createElement(ApprovalTree, {
        document: treeDoc,
        theme,
        accent,
        onChange: setTreeDoc,
      }),
    ),
    createElement('textarea', {
      className: 'demo-output',
      readOnly: true,
      value: output,
      placeholder: '点「保存」查看导出结果（BPMN XML / designer JSON）',
    }),
  );
}

createRoot(document.getElementById('root')!).render(createElement(App));
