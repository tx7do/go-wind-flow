/**
 * go-wind-flow 属性面板扩展：userTask 的 flow:assignees 可视化配置。
 *
 * 配置以 flow:assignees 扩展元素 body JSON 承载（与 Go 侧 format/bpmn、
 * format/designer、docs/designer_contract.md 同一契约）。
 */
import { TextFieldEntry, SelectEntry } from '@bpmn-io/properties-panel';

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

/** bpmn-js-properties-panel v5 的分组形状（宽松面）。 */
interface PanelGroup {
  id: string;
  label: string;
  entries: unknown[];
}

function entriesFor(env: PanelEnv, cfg: AssigneesConfig) {
  const { modeling, element, translate } = env;
  const set = (patch: Partial<AssigneesConfig>) => writeAssignees(modeling, element, { ...cfg, ...patch });

  return [
    TextFieldEntry({
      element,
      id: 'flow-rules',
      label: translate('办理人规则（JSON：kind=USER/STARTER/LEADER/POSITION/ROLE/GROUP/EXPR）'),
      getValue: () => JSON.stringify(cfg.approvers ?? []),
      setValue: (_el: FlowElement, v: unknown) => {
        try {
          const rules = JSON.parse(String(v ?? '[]')) as AssigneesConfig['approvers'];
          set({ approvers: Array.isArray(rules) ? rules : [] });
        } catch {
          /* 非法 JSON 忽略本次输入 */
        }
      },
      debounce: 500,
    }),
    SelectEntry({
      element,
      id: 'flow-strategy',
      label: translate('审批策略'),
      getValue: () => cfg.strategy ?? 'ALL',
      setValue: (_el: FlowElement, v: unknown) => set({ strategy: v === 'ALL' ? undefined : String(v) }),
      options: [
        { value: 'ALL', label: translate('会签（全部通过）') },
        { value: 'ANY', label: translate('或签（任一通过）') },
      ],
    }),
    SelectEntry({
      element,
      id: 'flow-reject',
      label: translate('驳回模式'),
      getValue: () => cfg.rejectMode ?? 'TERMINATE',
      setValue: (_el: FlowElement, v: unknown) => set({ rejectMode: v === 'STAY' ? 'STAY' : undefined }),
      options: [
        { value: 'TERMINATE', label: translate('驳回即终结') },
        { value: 'STAY', label: translate('驳回停留') },
      ],
    }),
    TextFieldEntry({
      element,
      id: 'flow-collection',
      label: translate('集合表达式（多实例，如 items）'),
      getValue: () => cfg.collection ?? '',
      setValue: (_el: FlowElement, v: unknown) => set({ collection: String(v ?? '') }),
    }),
    TextFieldEntry({
      element,
      id: 'flow-element',
      label: translate('项变量名（串行 collection，缺省 item）'),
      getValue: () => cfg.elementVar ?? '',
      setValue: (_el: FlowElement, v: unknown) => set({ elementVar: String(v ?? '') }),
    }),
    TextFieldEntry({
      element,
      id: 'flow-due',
      label: translate('任务超时（Go duration，如 24h）'),
      getValue: () => cfg.due ?? '',
      setValue: (_el: FlowElement, v: unknown) => set({ due: String(v ?? '') }),
    }),
    SelectEntry({
      element,
      id: 'flow-action',
      label: translate('超时动作'),
      getValue: () => cfg.action ?? '',
      setValue: (_el: FlowElement, v: unknown) => set({ action: v ? String(v) : undefined }),
      options: [
        { value: '', label: translate('仅广播事件（监听器自决）') },
        { value: 'auto_approve', label: translate('自动通过') },
        { value: 'auto_reject', label: translate('自动驳回') },
      ],
    }),
  ];
}

// ============================================================
// 属性面板 Provider（bpmn-js-properties-panel v5 getGroups API）
// ============================================================

interface PanelEnv {
  modeling: unknown;
  element: FlowElement;
  translate: (s: string) => string;
}

/** bpmn-js-properties-panel v5 的分组形状（宽松面）。 */
interface PanelGroup {
  id: string;
  label: string;
  entries: unknown[];
}

/** bpmn-js additionalModules：注册 go-wind-flow 属性面板 Provider。 */
export const FlowPropertiesModule = {
  __init__: ['flowPropertiesProvider'],
  flowPropertiesProvider: ["type", FlowPropertiesProvider],
};

/** $inject 显式声明：pre-bundle 压缩改写参数名，diagram-js 注入器按参数名解析会失败。 */
function FlowPropertiesProvider(
  this: Record<string, unknown>,
  modeling: unknown,
  propertiesPanel: PropertiesPanelInstance,
  translate: (s: string) => string,
): void {
  this.getGroups = function (element: FlowElement) {
    return function (entries: PanelGroup[]) {
      if (element.type !== 'bpmn:UserTask') return entries;
      if (!modeling) {
        // 理论上不可达（modeling 走 DI 注入）；失败可见化，不再静默吞组。
        console.warn('[go-wind-flow] properties panel: modeling service unavailable');
        return entries;
      }
      const cfg = readAssignees(element);
      return [
        {
          id: 'flow-assignees-group',
          label: translate('go-wind-flow 审批配置'),
          entries: entriesFor({ modeling, element, translate }, cfg) as unknown[],
        },
        ...entries,
      ];
    };
  };
  propertiesPanel.registerProvider(1500, this);
}

// modeling 经 diagram-js 注入器直接获取——不要读 propertiesPanel._modeling，
// bpmn-js-properties-panel v5 的服务实例上不存在该私有属性（会静默丢组）。
(FlowPropertiesProvider as unknown as { $inject: string[] }).$inject = ['modeling', 'propertiesPanel', 'translate'];
