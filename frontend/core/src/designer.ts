/**
 * go-wind-flow 流程设计器核心外观：bpmn-js 建模器 + go-wind-flow 属性面板。
 *
 * 用法（框架无关，Vue/React 适配器基于此封装）：
 *
 *   const d = createDesigner({ canvas: '#canvas', panel: '#panel' });
 *   await d.loadXML(bpmnXml);
 *   const xml = await d.getXML();   // 保存：POST 给 BFF → Deploy(format=bpmn-xml)
 *   d.destroy();
 */
import Modeler from 'bpmn-js/lib/Modeler';
import { BpmnPropertiesPanelModule, BpmnPropertiesProviderModule } from 'bpmn-js-properties-panel';
import type Modeling from 'bpmn-js/lib/features/modeling/Modeling';

import { FlowModdleDescriptor, FlowPropertiesModule } from './panel';
import { documentToXML, xmlToDocument } from './convert';
import { applyAccent, applyTheme, ensureStyles, designerCSS, type Theme } from './theme';
import type { DesignerDocument } from './types';

export interface DesignerOptions {
  /** 画布容器（元素或选择器）。 */
  canvas: string | HTMLElement;
  /** 属性面板容器。 */
  panel: string | HTMLElement;
  /** 主题：亮色 / 暗色 / 跟随系统（缺省 auto）。 */
  theme?: Theme;
  /** 自定义主题色（--gwf-accent，弱色/高亮经 color-mix 自动派生）。 */
  accent?: string;
}

export interface DesignerHandle {
  /** 加载 BPMN XML。 */
  loadXML(xml: string): Promise<void>;
  /** 导出当前画布 BPMN XML（保存 → BFF → Deploy(format=bpmn-xml)）。 */
  getXML(): Promise<string>;
  /** 加载 designer-json-v1 文档（画布渲染为合成 DI 的 BPMN）。 */
  loadDocument(doc: DesignerDocument): Promise<void>;
  /** 导出 designer-json-v1 文档（从画布当前状态）。 */
  getDocument(key?: string): Promise<DesignerDocument>;
  /** 建模 API（宿主需要自定义交互时使用）。 */
  modeling(): Modeling;
  zoom(factor: number | 'fit-viewport'): void;
  /** 切换主题（light / dark / auto）。 */
  setTheme(theme: Theme): void;
  /** 自定义主题色。 */
  setAccent(color: string): void;
  destroy(): void;
}

function resolveEl(ref: string | HTMLElement): HTMLElement {
  return typeof ref === 'string' ? (document.querySelector(ref) as HTMLElement) : ref;
}

let stylesInjected = false;
function injectStylesOnce(): void {
  if (stylesInjected) return;
  const style = document.createElement('style');
  style.id = 'gwf-designer-styles';
  style.textContent = designerCSS;
  document.head.appendChild(style);
  stylesInjected = true;
}

export function createDesigner(opts: DesignerOptions): DesignerHandle {
  const canvas = resolveEl(opts.canvas);
  const panel = resolveEl(opts.panel);
  canvas.classList.add("gwf-designer"); // 主题选择器以 .gwf-designer 为根
  injectStylesOnce();
  applyTheme(canvas, opts.theme ?? 'auto');
  if (opts.accent) applyAccent(canvas, opts.accent);

  const modeler = new Modeler({
    container: canvas,
    keyboard: { bindTo: document },
    additionalModules: [
      BpmnPropertiesPanelModule,
      BpmnPropertiesProviderModule,
      FlowPropertiesModule,
    ] as Record<string, unknown>[],
    moddleExtensions: { flow: FlowModdleDescriptor },
    propertiesPanel: { parent: panel },
  });

  const modeling = (): Modeling => modeler.get<Modeling>('modeling');

  return {
    async loadXML(xml: string) {
      await modeler.importXML(xml);
    },
    async getXML(): Promise<string> {
      const { xml } = await modeler.saveXML({ format: true });
      return xml ?? '';
    },
    async loadDocument(doc: DesignerDocument) {
      await modeler.importXML(documentToXML(doc));
    },
    async getDocument(key?: string): Promise<DesignerDocument> {
      const { xml } = await modeler.saveXML({ format: true });
      return xmlToDocument(key ?? '', xml ?? '');
    },
    modeling() {
      return modeling();
    },
    zoom(factor: number | 'fit-viewport') {
      modeler.get<{ zoom: (f: number | 'fit-viewport') => void }>('canvas').zoom(factor);
    },
    setTheme(theme: Theme) {
      applyTheme(canvas, theme);
    },
    setAccent(color: string) {
      applyAccent(canvas, color);
    },
    destroy() {
      modeler.destroy();
    },
  };
}

export { documentToXML, xmlToDocument } from './convert';
export { FlowModdleDescriptor, readAssignees, writeAssignees } from './panel';
export { applyAccent, applyTheme, applyThemeToAll, designerCSS, ensureStyles, THEME_ATTR, type Theme } from './theme';
export * from './types';
