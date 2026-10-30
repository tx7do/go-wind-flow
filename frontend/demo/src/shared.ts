/** 示例流程与共享 UI 逻辑（Vue / React 两个 demo 页共用）。 */

export const leaveApprovalXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flow="https://github.com/tx7do/go-wind-flow"
             targetNamespace="https://github.com/tx7do/go-wind-flow">
  <process id="leave" name="请假审批" isExecutable="true">
    <startEvent id="start" name="提交申请"/>
    <userTask id="leader" name="直属领导审批">
      <extensionElements>
        <flow:assignees>{"approvers":[{"kind":"LEADER"}],"strategy":"ALL"}</flow:assignees>
      </extensionElements>
    </userTask>
    <exclusiveGateway id="gw" name="是否超过3天" default="gwd"/>
    <userTask id="director" name="总监审批">
      <extensionElements>
        <flow:assignees>{"approvers":[{"kind":"USER","value":"u_1001"}],"strategy":"ANY","due":"48h","action":"auto_approve"}</flow:assignees>
      </extensionElements>
    </userTask>
    <endEvent id="end" name="结束"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="leader"/>
    <sequenceFlow id="f2" sourceRef="leader" targetRef="gw"/>
    <sequenceFlow id="f3" sourceRef="gw" targetRef="director">
      <conditionExpression xsi:type="tFormalExpression" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">$&#123;days &amp;gt; 3&#125;</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="gwd" sourceRef="gw" targetRef="end"/>
    <sequenceFlow id="f4" sourceRef="director" targetRef="end"/>
  </process>
</definitions>`;

/** 审批树示例文档（含条件分支）。 */
export const treeSampleDoc = {
  version: 1 as const,
  key: 'tree_demo',
  name: '报销审批（树）',
  nodes: [
    { id: 'start', type: 'startEvent' as const, name: '提交', ui: { x: 100, y: 200 } },
    {
      id: 'finance',
      type: 'userTask' as const,
      name: '财务初审',
      approver: { rules: [{ kind: 'GROUP' as const, value: 'finance' }], strategy: 'ALL' as const },
      ui: { x: 300, y: 200 },
    },
    { id: 'gw', type: 'exclusiveGateway' as const, name: '金额', defaultEdge: 'bd', ui: { x: 500, y: 200 } },
    {
      id: 'director',
      type: 'userTask' as const,
      name: '总监审批',
      approver: { rules: [{ kind: 'USER', value: 'u_1001' }], strategy: 'ANY' as const },
      ui: { x: 700, y: 100 },
    },
    { id: 'end', type: 'endEvent' as const, ui: { x: 900, y: 200 } },
  ],
  edges: [
    { id: 'f1', source: 'start', target: 'finance' },
    { id: 'f2', source: 'finance', target: 'gw' },
    { id: 'bh', source: 'gw', target: 'director', name: '超1000', condition: 'amount > 1000' },
    { id: 'bd', source: 'gw', target: 'end', isDefault: true },
    { id: 'f3', source: 'director', target: 'end' },
  ],
};

export type ThemeMode = 'light' | 'dark' | 'auto';

export interface ToolbarBindings {
  root: HTMLElement;
  onTheme: (t: ThemeMode) => void;
  onAccent: (c: string) => void;
}

/** 渲染工具条（主题三态 + 主题色 + 保存），返回卸载函数。 */
export function renderToolbar(bind: ToolbarBindings): () => void {
  const bar = document.createElement('div');
  bar.className = 'demo-toolbar';
  bar.innerHTML = `
    <span class="demo-brand">go-wind-flow 设计器 Demo</span>
    <span class="demo-group" data-role="theme">
      <button data-t="light">亮色</button>
      <button data-t="dark">暗色</button>
      <button data-t="auto">跟随系统</button>
    </span>
    <label class="demo-accent">主题色 <input type="color" value="#00add8" /></label>
    <button class="demo-save">保存</button>
  `;
  bind.root.prepend(bar);

  const onThemeClick = (ev: Event) => {
    const t = (ev.target as HTMLElement).dataset.t as ThemeMode | undefined;
    if (t) bind.onTheme(t);
  };
  const onAccent = (ev: Event) => bind.onAccent((ev.target as HTMLInputElement).value);
  bar.querySelector('[data-role="theme"]')?.addEventListener('click', onThemeClick);
  bar.querySelector('.demo-accent input')?.addEventListener('input', onAccent);

  return () => bar.remove();
}

/** 注入 demo 页自身的布局样式（独立于组件 token）。 */
export function injectDemoStyles(): void {
  if (document.getElementById('gwf-demo-styles')) return;
  const style = document.createElement('style');
  style.id = 'gwf-demo-styles';
  style.textContent = `
    body { margin: 0; font-family: system-ui, -apple-system, 'Segoe UI', 'PingFang SC', sans-serif; }
    .demo-page { max-width: 1280px; margin: 0 auto; padding: 16px; }
    .demo-toolbar { display: flex; gap: 12px; align-items: center; padding: 10px 0 14px; flex-wrap: wrap; }
    .demo-brand { font-weight: 700; margin-right: 8px; }
    .demo-group { display: inline-flex; gap: 0; }
    .demo-group button { padding: 4px 12px; border: 1px solid #d0d3d9; background: #fff; cursor: pointer; }
    .demo-group button + button { border-left: none; }
    .demo-group button:first-child { border-radius: 6px 0 0 6px; }
    .demo-group button:last-child { border-radius: 0 6px 6px 0; }
    .demo-accent { display: inline-flex; gap: 6px; align-items: center; }
    .demo-save { padding: 4px 14px; border: none; border-radius: 6px; background: #00add8; color: #fff; cursor: pointer; }
    .demo-tabs { display: flex; gap: 0; margin-bottom: 12px; }
    .demo-tabs button { padding: 6px 16px; border: 1px solid #d0d3d9; background: #fff; cursor: pointer; }
    .demo-tabs button.active { background: #00add8; color: #fff; border-color: #00add8; }
    .demo-output { width: 100%; box-sizing: border-box; height: 180px; margin-top: 12px; font-family: ui-monospace, monospace; font-size: 12px; }
    .demo-canvas-wrap { display: flex; gap: 8px; }
    .demo-canvas-wrap > .gwf-canvas { flex: 1; border: 1px solid #d0d3d9; height: 560px; }
    .demo-canvas-wrap > .gwf-panel { width: 320px; border: 1px solid #d0d3d9; height: 560px; overflow-y: auto; }
    @media (prefers-color-scheme: dark) {
      body { background: #14161a; color: #e5e7eb; }
      .demo-group button { background: #1e2228; border-color: #2c313a; color: #e5e7eb; }
      .demo-tabs button { background: #1e2228; border-color: #2c313a; color: #e5e7eb; }
      .demo-tabs button.active { background: #00add8; }
      .demo-save { background: #00add8; }
      .demo-output { background: #191c21; color: #e5e7eb; border: 1px solid #2c313a; }
    }
  `;
  document.head.appendChild(style);
}
