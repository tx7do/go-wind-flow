/**
 * go-wind-flow 属性面板扩展：userTask 的 flow:assignees 可视化配置。
 *
 * 配置以 flow:assignees 扩展元素 body JSON 承载（与 Go 侧 format/bpmn、
 * format/designer、docs/designer_contract.md 同一契约）。
 */
import type { AssigneesConfig } from './types';
import type { BpmnJSModdleElement, FlowElement, PropertiesPanelInstance } from './moddle';

const GROUP_ID = 'flow-assignees-group';

/** flow 命名空间 moddle 扩展（注册进 Modeler moddleExtensions.flow）。 */
export const FlowModdleDescriptor = {
  name: 'flow',
  uri: 'https://github.com/tx7do/go-wind-flow',
  prefix: 'flow',
  xml: { tagAlias: 'lowerCase' },
  types: [
    {
      name: 'assignees',
      extends: ['bpmn:BaseElement'],
      properties: [{ name: 'value', isBody: true, type: 'String' }],
    },
  ],
} as const;

/** 读取元素上的 flow:assignees 配置（无则返回空对象）。 */
export function readAssignees(element: FlowElement): AssigneesConfig {
  const ext = element.extensionElements?.values as BpmnJSModdleElement[] | undefined ?? [];
  for (const v of ext ?? []) {
    if (v.$type === 'flow:assignees') {
      try {
        return JSON.parse(String(v.value ?? '{}')) as AssigneesConfig;
      } catch {
        return {};
      }
    }
  }
  return {};
}

/** 写回 flow:assignees 配置（空对象时移除扩展元素）。 */
export function writeAssignees(modeling: unknown, element: FlowElement, cfg: AssigneesConfig): void {
  const m = modeling as {
    updateProperties(el: unknown, props: Record<string, unknown>): void;
    updateModdleProperties(el: unknown, moddleEl: unknown, props: Record<string, unknown>): void;
  };
  const ext = element.extensionElements?.values as BpmnJSModdleElement[] | undefined ?? [];
  const existing = ext?.find((v: BpmnJSModdleElement) => v.$type === 'flow:assignees');
  const empty = Object.keys(cfg).length === 0;
  const body = empty ? undefined : JSON.stringify(cfg);

  if (existing) {
    if (body === undefined) {
      m.updateModdleProperties(element, element.extensionElements, {
        values: ext.filter((v) => v !== existing),
      });
    } else {
      m.updateModdleProperties(element, existing, { value: body });
    }
    return;
  }
  if (body === undefined) return;

  const parent = element.extensionElements;
  if (!parent) {
    m.updateProperties(element, {
      extensionElements: element.$model.create('bpmn:ExtensionElements', {
        values: [element.$model.create('flow:assignees', { value: body })],
      }),
    });
    return;
  }
  m.updateModdleProperties(element, parent, {
    values: [...ext, element.$model.create('flow:assignees', { value: body })],
  });
}

// ============================================================
// 属性面板 Provider（bpmn-js-properties-panel v5 getGroups API）
// ============================================================

interface PanelEnv {
  modeling: unknown;
  element: FlowElement;
  translate: (s: string) => string;
}

interface PanelEntry {
  id: string;
  label: string;
  type?: string;
  element?: FlowElement;
  getValue?: () => string | boolean;
  setValue?: (element: FlowElement, v: string | boolean | undefined) => void;
  options?: { value: string; label: string }[];
  debounce?: number;
}

function entriesFor(env: PanelEnv, cfg: AssigneesConfig): PanelEntry[] {
  const { modeling, element, translate } = env;
  const set = (patch: Partial<AssigneesConfig>) => writeAssignees(modeling, element, { ...cfg, ...patch });

  const text = (id: string, label: string, get: () => string, apply: (v: string) => void): PanelEntry => ({
    id,
    label: translate(label),
    type: 'textfield',
    element,
    getValue: () => get(),
    setValue: (_el, v) => apply(String(v ?? '')),
    debounce: 300,
  });
  const select = (
    id: string,
    label: string,
    options: { value: string; label: string }[],
    get: () => string,
    apply: (v: string) => void,
  ): PanelEntry => ({
    id,
    label: translate(label),
    type: 'select',
    element,
    getValue: () => get(),
    setValue: (_el, v) => apply(String(v ?? '')),
    options,
  });

  const entries: PanelEntry[] = [];

  entries.push({
    id: 'flow-rules',
    label: translate('办理人规则（JSON：kind=USER/STARTER/LEADER/POSITION/ROLE/GROUP/EXPR）'),
    type: 'textfield', // v1：JSON 直编（可视化表单在后续版本跟进）
    element,
    getValue: () => JSON.stringify(cfg.approvers ?? []),
    setValue: (_el, v) => {
      try {
        const rules = JSON.parse(String(v ?? '[]')) as AssigneesConfig['approvers'];
        set({ approvers: Array.isArray(rules) ? rules : [] });
      } catch {
        /* 非法 JSON 忽略本次输入 */
      }
    },
    debounce: 500,
  });

  entries.push(
    select(
      'flow-strategy',
      '审批策略',
      [
        { value: 'ALL', label: '会签（全部通过）' },
        { value: 'ANY', label: '或签（任一通过）' },
      ],
      () => cfg.strategy ?? 'ALL',
      (v) => set({ strategy: v === 'ALL' ? undefined : v }),
    ),
    select(
      'flow-reject',
      '驳回模式',
      [
        { value: 'TERMINATE', label: '驳回即终结' },
        { value: 'STAY', label: '驳回停留' },
      ],
      () => cfg.rejectMode ?? 'TERMINATE',
      (v) => set({ rejectMode: v === 'TERMINATE' ? undefined : v }),
    ),
    text('flow-collection', '集合表达式（多实例，如 items）', () => cfg.collection ?? '', (v) => set({ collection: v })),
    text('flow-element', '项变量名（串行 collection，缺省 item）', () => cfg.elementVar ?? '', (v) => set({ elementVar: v })),
    text('flow-due', '任务超时（Go duration，如 24h）', () => cfg.due ?? '', (v) => set({ due: v })),
    select(
      'flow-action',
      '超时动作',
      [
        { value: '', label: '仅广播事件（监听器自决）' },
        { value: 'auto_approve', label: '自动通过' },
        { value: 'auto_reject', label: '自动驳回' },
      ],
      () => cfg.action ?? '',
      (v) => set({ action: v }),
    ),
  );

  return entries;
}

/** bpmn-js additionalModules：注册 go-wind-flow 属性面板 Provider。 */
export const FlowPropertiesModule = {
  __init__: ['flowPropertiesProvider'],
  flowPropertiesProvider: ["type", FlowPropertiesProvider],
};

/** $inject 显式声明：pre-bundle 压缩改写参数名，diagram-js 注入器按参数名解析会失败。 */
function FlowPropertiesProvider(this: Record<string, unknown>, propertiesPanel: PropertiesPanelInstance, translate: (s: string) => string): void {
  this.getGroups = function (element: FlowElement) {
    return function (entries: PanelEntry[][]) {
      if (element.type !== 'bpmn:UserTask') return entries;
      const modeling = propertiesPanel._modeling;
      if (!modeling) return entries;
      const cfg = readAssignees(element);
      return [
        {
          id: 'flow-assignees-group',
          label: translate('go-wind-flow 审批配置'),
          entries: entriesFor({ modeling, element, translate }, cfg),
        },
        ...entries,
      ];
    };
  };
  propertiesPanel.registerProvider(1500, this);
}

(FlowPropertiesProvider as unknown as { $inject: string[] }).$inject = ['propertiesPanel', 'translate'];
