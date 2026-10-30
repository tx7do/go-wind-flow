import type { DefineComponent } from 'vue';
import type { DesignerDocument, Theme } from '@go-wind/flow-designer-core';

export interface ApprovalTreeProps {
  document: DesignerDocument;
  theme?: Theme;
  accent?: string;
}

/** 垂直审批树（钉钉式业务模式）。emit：change(document) / unsupported(reason)。 */
export declare const ApprovalTree: DefineComponent<
  ApprovalTreeProps,
  Record<string, never>,
  Record<string, never>,
  Record<string, never>,
  Record<string, never>,
  Record<string, never>,
  { change: (doc: DesignerDocument) => void; unsupported: (reason: string) => void }
>;
