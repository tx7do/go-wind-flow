import type { DefineComponent } from 'vue';
import type { DesignerDocument, DesignerHandle } from '@go-wind/flow-designer-core';

export interface FlowDesignerProps {
  xml?: string;
  document?: DesignerDocument;
  height?: string;
}

/** go-wind-flow 流程设计器（Vue3）。expose：getXML / getDocument。 */
export declare const FlowDesigner: DefineComponent<
  FlowDesignerProps,
  { getXML(): Promise<string>; getDocument(key?: string): Promise<DesignerDocument> },
  Record<string, never>,
  Record<string, never>,
  Record<string, never>,
  Record<string, never>,
  { ready: (handle: DesignerHandle) => void; error: (err: unknown) => void }
>;
