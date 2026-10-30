/**
 * bpmn-js / properties-panel 的最小类型面。
 * 上游类型随版本变动较大，这里只声明本库实际使用的 API 面（宽松 any），
 * 保证适配层类型稳定；宿主项目如已有精确类型可覆盖。
 */

/** bpmn 元素 + moddle 扩展的最小结构面（本库只用到这里列出的属性）。 */
export interface FlowElement {
  id: string;
  type: string;
  $type: string;
  assignee?: string;
  extensionElements?: { values?: any[] };
  $model: { create(type: string, attrs?: Record<string, unknown>): any };
  businessObject?: any;
  [key: string]: any;
}

/** moddle 扩展元素的最小面（flow:assignees 等）。 */
export interface BpmnJSModdleElement {
  $type: string;
  value?: string;
  [key: string]: unknown;
}

declare module 'bpmn-js-properties-panel' {
  export const BpmnPropertiesPanelModule: unknown;
  export const BpmnPropertiesProviderModule: unknown;
}

declare module 'bpmn-js/lib/Modeler' {
  export default class Modeler {
    constructor(options: Record<string, unknown>);
    importXML(xml: string): Promise<{ warnings: unknown[] }>;
    saveXML(options?: Record<string, unknown>): Promise<{ xml?: string }>;
    get<T = unknown>(name: string): T;
    destroy(): void;
  }
}

declare module 'bpmn-js/lib/features/modeling/Modeling' {
  export default interface Modeling {
    updateProperties(element: unknown, props: Record<string, unknown>): void;
    updateModdleProperties(element: unknown, moddleElement: unknown, props: Record<string, unknown>): void;
  }
}

/** properties-panel 实例的最小面（_modeling 为上游内部字段）。 */
export interface PropertiesPanelInstance {
  registerProvider(priority: number, provider: unknown): void;
  _modeling?: unknown;
  _eventBus?: { on(event: string, cb: (e: unknown) => void): void };
}
