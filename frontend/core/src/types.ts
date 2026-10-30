/**
 * designer-json-v1 契约类型（与 docs/designer_contract.md 对齐，
 * 与 Go 侧 format/designer 的 Document/Node/Edge 一一对应）。
 */

export interface Position {
  x: number;
  y: number;
}

export interface EdgeUI {
  waypoints?: [number, number][];
}

export interface ApproverRule {
  kind: 'USER' | 'STARTER' | 'LEADER' | 'POSITION' | 'ROLE' | 'EXPR' | 'GROUP';
  value?: string;
}

export interface ApproverSpec {
  rules?: ApproverRule[];
  strategy?: 'ALL' | 'ANY';
  rejectMode?: 'TERMINATE' | 'STAY';
  sequential?: boolean;
  collection?: string;
  elementVar?: string;
}

export interface TimerSpec {
  duration?: string;
  action?: '' | 'auto_approve' | 'auto_reject';
}

export interface ScriptSpec {
  lang?: string;
  code?: string;
  scope?: '' | 'instance' | 'local';
}

export type NodeType =
  | 'startEvent'
  | 'endEvent'
  | 'userTask'
  | 'scriptTask'
  | 'serviceTask'
  | 'intermediateTimerEvent'
  | 'intermediateMessageEvent'
  | 'boundaryEvent'
  | 'errorEndEvent'
  | 'exclusiveGateway'
  | 'parallelGateway'
  | 'inclusiveGateway'
  | 'callActivity';

export interface DesignerNode {
  id: string;
  type: NodeType;
  name?: string;
  approver?: ApproverSpec;
  timer?: TimerSpec;
  script?: ScriptSpec;
  subprocessKey?: string;
  subprocessVersion?: number;
  message?: string;
  attachedTo?: string;
  interrupting?: boolean;
  errorCode?: string;
  defaultEdge?: string;
  documentation?: string;
  ui?: Position;
}

export interface DesignerEdge {
  id: string;
  source: string;
  target: string;
  name?: string;
  condition?: string;
  isDefault?: boolean;
  ui?: EdgeUI;
}

export interface DesignerDocument {
  version: 1;
  key: string;
  name?: string;
  nodes: DesignerNode[];
  edges: DesignerEdge[];
}

/** flow:assignees 扩展元素在面板中的可编辑配置（body JSON）。 */
export interface AssigneesConfig {
  approvers?: ApproverRule[];
  strategy?: string;
  rejectMode?: string;
  sequential?: boolean;
  collection?: string;
  elementVar?: string;
  due?: string;
  action?: string;
}
