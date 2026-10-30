/**
 * designer-json-v1 ↔ BPMN XML 双向转换（纯字符串/DOM，无 bpmn-js 依赖）。
 *
 * JSON → XML 时合成简单 DI（分层网格布局），保证 bpmn-js 可直接渲染；
 * XML → JSON 时读取 flow:assignees 扩展与 bpmndi 坐标。
 * 嵌入子流程按后端 SES 扁平化语义处理（导出不还原嵌套，与 Go 侧一致）。
 */
import type { DesignerDocument, DesignerEdge, DesignerNode, AssigneesConfig, ApproverSpec, TimerSpec } from './types';

const BPMN_NS = 'http://www.omg.org/spec/BPMN/20100524/MODEL';

// ============================================================
// designer JSON → BPMN XML
// ============================================================

export function documentToXML(doc: DesignerDocument): string {
  const esc = (s: string) =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  const attr = (name: string, v: string | undefined) => (v !== undefined && v !== '' ? ` ${name}="${esc(String(v))}"` : '');

  const layout = computeLayout(doc);
  const parts: string[] = [];
  parts.push('<?xml version="1.0" encoding="UTF-8"?>');
  parts.push(
    `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"` +
      ` xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"` +
      ` xmlns:dc="http://www.omg.org/spec/DD/20100524/DC"` +
      ` xmlns:flow="https://github.com/tx7do/go-wind-flow"` +
      ` targetNamespace="https://github.com/tx7do/go-wind-flow">`,
  );
  parts.push(`  <process id="${esc(doc.key)}" name="${esc(doc.name ?? '')}" isExecutable="true">`);

  const order: Record<string, number> = {
    startEvent: 0, endEvent: 1, intermediateTimerEvent: 2, intermediateMessageEvent: 3,
    boundaryEvent: 4, userTask: 5, scriptTask: 6, serviceTask: 7,
    exclusiveGateway: 8, parallelGateway: 9, inclusiveGateway: 10, callActivity: 11, errorEndEvent: 12,
  };
  const nodes = [...doc.nodes].sort((a, b) => (order[a.type] ?? 99) - (order[b.type] ?? 99) || a.id.localeCompare(b.id));

  for (const n of nodes) {
    if (n.type === 'boundaryEvent') {
      const trigger = n.errorCode
        ? `      <errorEventDefinition errorRef="${esc(n.errorCode)}"/>`
        : `      <timerEventDefinition><timeDuration>${esc(isoOf(n.timer))}</timeDuration></timerEventDefinition>`;
      parts.push(
        `    <boundaryEvent id="${esc(n.id)}" name="${esc(n.name ?? '')}" attachedToRef="${esc(n.attachedTo ?? '')}"` +
          ` cancelActivity="${n.interrupting === false ? 'false' : 'true'}">`,
        trigger,
        '    </boundaryEvent>',
      );
      continue;
    }
    if (n.type === 'errorEndEvent') {
      parts.push(
        `    <endEvent id="${esc(n.id)}" name="${esc(n.name ?? '')}">`,
        `      <errorEventDefinition errorRef="${esc(n.errorCode ?? '')}"/>`,
        '    </endEvent>',
      );
      continue;
    }
    if (n.type === 'intermediateTimerEvent' || n.type === 'intermediateMessageEvent') {
      parts.push(`    <intermediateCatchEvent id="${esc(n.id)}" name="${esc(n.name ?? '')}">`);
      if (n.type === 'intermediateTimerEvent') {
        parts.push(`      <timerEventDefinition><timeDuration>${esc(isoOf(n.timer))}</timeDuration></timerEventDefinition>`);
      } else {
        parts.push(`      <messageEventDefinition messageRef="${esc(n.message ?? '')}"/>`);
      }
      parts.push('    </intermediateCatchEvent>');
      continue;
    }
    if (n.type === 'userTask') {
      const cfg = toAssignees(n.approver, n.timer);
      if (Object.keys(cfg).length > 0) {
        parts.push(
          `    <userTask id="${esc(n.id)}" name="${esc(n.name ?? '')}">`,
          '      <extensionElements>',
          `        <flow:assignees>${esc(JSON.stringify(cfg))}</flow:assignees>`,
          '      </extensionElements>',
          '    </userTask>',
        );
        continue;
      }
    }
    if (n.type === 'scriptTask' || n.type === 'serviceTask') {
      const scope = n.script?.scope === 'local' ? ' flow:scope="local"' : '';
      const lang = n.script?.lang ? ` scriptFormat="${esc(n.script.lang)}"` : '';
      parts.push(
        `    <${n.type} id="${esc(n.id)}" name="${esc(n.name ?? '')}"${lang}${scope}>`,
        n.script?.code ? `      <script><![CDATA[${n.script.code}]]></script>` : '',
        `    </${n.type}>`,
      );
      continue;
    }
    if (n.type === 'callActivity') {
      parts.push(`    <callActivity id="${esc(n.id)}" name="${esc(n.name ?? '')}" calledElement="${esc(n.subprocessKey ?? '')}"/>`);
      continue;
    }
    // startEvent / endEvent / gateways
    const defAttr = n.type === 'exclusiveGateway' && n.defaultEdge ? ` default="${esc(n.defaultEdge)}"` : '';
    parts.push(`    <${n.type} id="${esc(n.id)}" name="${esc(n.name ?? '')}"${defAttr}/>`);
  }

  for (const e of doc.edges) {
    const cond = e.condition ? `<conditionExpression xsi:type="tFormalExpression">${esc(e.condition)}</conditionExpression>` : '';
    parts.push(`    <sequenceFlow id="${esc(e.id)}" sourceRef="${esc(e.source)}" targetRef="${esc(e.target)}">${cond}</sequenceFlow>`);
  }

  parts.push('  </process>');
  parts.push('  <bpmndi:BPMNDiagram id="BPMNDiagram_1">');
  parts.push('    <bpmndi:BPMNPlane id="BPMNPlane_1" bpmnElement="' + esc(doc.key) + '">');
  for (const n of nodes) {
    const p = layout.nodes[n.id] ?? { x: 160, y: 160 };
    const w = n.type === 'userTask' || n.type === 'callActivity' ? 100 : n.type === 'startEvent' || n.type === 'endEvent' ? 36 : 50;
    const h = n.type === 'userTask' || n.type === 'callActivity' ? 80 : n.type === 'startEvent' || n.type === 'endEvent' ? 36 : 50;
    parts.push(
      `      <bpmndi:BPMNShape id="${esc(n.id)}_di" bpmnElement="${esc(n.id)}">`,
      `        <dc:Bounds x="${p.x}" y="${p.y}" width="${w}" height="${h}"/>`,
      '      </bpmndi:BPMNShape>',
    );
  }
  for (const e of doc.edges) {
    const a = layout.nodes[e.source];
    const b = layout.nodes[e.target];
    if (!a || !b) continue;
    parts.push(
      `      <bpmndi:BPMNEdge id="${esc(e.id)}_di" bpmnElement="${esc(e.id)}">`,
      `        <di:waypoint x="${a.x + 50}" y="${a.y + 40}"/>`,
      `        <di:waypoint x="${b.x}" y="${b.y + 40}"/>`,
      '      </bpmndi:BPMNEdge>',
    );
  }
  parts.push('    </bpmndi:BPMNPlane>');
  parts.push('  </bpmndi:BPMNDiagram>');
  parts.push('</definitions>');
  return parts.filter((s) => s !== '').join('\n') + '\n';

  function isoOf(t?: TimerSpec): string {
    const ms = t?.duration ? durationStringToMs(t.duration) : 0;
    return msToISO(ms);
  }
}

function durationStringToMs(s: string): number {
  const m = /^((\d+)h)?((\d+)m)?((\d+)s)?((\d+)ms)?$/.exec(s.trim());
  if (!m) return 0;
  return (+(m[2] ?? 0)) * 3600_000 + (+(m[4] ?? 0)) * 60_000 + (+(m[6] ?? 0)) * 1000 + (+(m[8] ?? 0));
}

function msToISO(ms: number): string {
  if (ms <= 0) return 'PT1S';
  const h = Math.floor(ms / 3600_000);
  const m = Math.floor((ms % 3600_000) / 60_000);
  const s = Math.round((ms % 60_000) / 1000);
  let out = 'PT';
  if (h) out += `${h}H`;
  if (m) out += `${m}M`;
  if (s || (!h && !m)) out += `${s}S`;
  return out;
}

/** 简单分层布局：按最长路径分层，层内纵向排列。 */
export function computeLayout(doc: DesignerDocument): { nodes: Record<string, { x: number; y: number }> } {
  const layer = new Map<string, number>();
  const byId = new Map(doc.nodes.map((n) => [n.id, n]));
  const incoming = new Map<string, number>();
  for (const e of doc.edges) incoming.set(e.target, (incoming.get(e.target) ?? 0) + 1);

  const queue: string[] = [];
  for (const n of doc.nodes) {
    if ((incoming.get(n.id) ?? 0) === 0 || n.type === 'startEvent') {
      layer.set(n.id, 0);
      queue.push(n.id);
    }
  }
  while (queue.length) {
    const cur = queue.shift()!;
    for (const e of doc.edges.filter((x) => x.source === cur)) {
      const next = Math.max(layer.get(e.target) ?? 0, layer.get(cur)! + 1);
      if (!layer.has(e.target) || next > (layer.get(e.target) ?? 0)) {
        layer.set(e.target, next);
        if (byId.has(e.target)) queue.push(e.target);
      }
    }
  }
  const perLayer = new Map<number, number>();
  const nodes: Record<string, { x: number; y: number }> = {};
  for (const n of doc.nodes) {
    const l = layer.get(n.id) ?? 0;
    const row = perLayer.get(l) ?? 0;
    perLayer.set(l, row + 1);
    nodes[n.id] = { x: 160 + l * 200, y: 100 + row * 140 };
  }
  return { nodes };
}

function toAssignees(approver?: ApproverSpec, timer?: TimerSpec): AssigneesConfig {
  const cfg: AssigneesConfig = {};
  if (approver?.rules?.length) cfg.approvers = approver.rules;
  if (approver?.strategy === 'ANY') cfg.strategy = 'ANY';
  if (approver?.rejectMode === 'STAY') cfg.rejectMode = 'STAY';
  if (approver?.sequential) cfg.sequential = true;
  if (approver?.collection) cfg.collection = approver.collection;
  if (approver?.elementVar) cfg.elementVar = approver.elementVar;
  if (timer?.duration) cfg.due = timer.duration;
  if (timer?.action) cfg.action = timer.action;
  return cfg;
}

// ============================================================
// BPMN XML → designer JSON
// ============================================================

export function xmlToDocument(key: string, xml: string): DesignerDocument {
  const doc = new DOMParser().parseFromString(xml, 'text/xml');
  const proc = doc.getElementsByTagNameNS(BPMN_NS, 'process')[0];
  if (!proc) throw new Error('no <process> in BPMN XML');
  const keyAttr = proc.getAttribute('id') ?? key;

  const nodes: DesignerNode[] = [];
  const edges: DesignerEdge[] = [];
  const tagTypes: Record<string, DesignerNode['type']> = {
    startEvent: 'startEvent',
    endEvent: 'endEvent',
    userTask: 'userTask',
    scriptTask: 'scriptTask',
    serviceTask: 'serviceTask',
    intermediateCatchEvent: 'intermediateTimerEvent',
    boundaryEvent: 'boundaryEvent',
    exclusiveGateway: 'exclusiveGateway',
    parallelGateway: 'parallelGateway',
    inclusiveGateway: 'inclusiveGateway',
    callActivity: 'callActivity',
  };

  const child = (el: Element, tag: string): Element | null => {
    for (let i = 0; i < el.children.length; i++) {
      const c = el.children[i];
      if (c.localName === tag) return c;
    }
    return null;
  };

  const parseNode = (el: Element, type: DesignerNode['type']) => {
    const id = el.getAttribute('id')!;
    const n: DesignerNode = { id, type, name: el.getAttribute('name') ?? undefined };

    const ext = child(el, 'extensionElements');
    const assignees = ext ? child(ext, 'assignees') : null;
    if (assignees && assignees.namespaceURI === 'https://github.com/tx7do/go-wind-flow') {
      try {
        const cfg = JSON.parse(assignees.textContent ?? '{}') as AssigneesConfig;
        const approver: ApproverSpec = {};
        if (cfg.approvers) approver.rules = cfg.approvers;
        if (cfg.strategy === 'ANY') approver.strategy = 'ANY';
        if (cfg.rejectMode === 'STAY') approver.rejectMode = 'STAY';
        if (cfg.sequential) approver.sequential = true;
        if (cfg.collection) approver.collection = cfg.collection;
        if (cfg.elementVar) approver.elementVar = cfg.elementVar;
        if (Object.keys(approver).length) n.approver = approver;
        if (cfg.due || cfg.action) n.timer = { duration: cfg.due, action: (cfg.action as TimerSpec['action']) ?? '' };
      } catch {
        /* 非法扩展 JSON：保留节点，忽略配置 */
      }
    }

    if (type === 'intermediateTimerEvent') {
      n.type = 'intermediateMessageEvent';
      const msg = child(el, 'messageEventDefinition');
      if (msg) {
        n.message = msg.getAttribute('messageRef') ?? '';
      } else {
        n.type = 'intermediateTimerEvent';
        const dur = child(el, 'timeDuration');
        n.timer = { duration: msToISO(durationStringToMs(dur?.textContent ?? '')) };
      }
    }
    if (type === 'boundaryEvent') {
      n.attachedTo = el.getAttribute('attachedToRef') ?? '';
      n.interrupting = (el.getAttribute('cancelActivity') ?? 'true') !== 'false';
      const err = child(el, 'errorEventDefinition');
      if (err) {
        n.errorCode = err.getAttribute('errorRef') ?? '';
      } else {
        const dur = child(el, 'timeDuration');
        n.timer = { duration: msToISO(durationStringToMs(dur?.textContent ?? '')) };
      }
    }
    if (type === 'endEvent') {
      const err = child(el, 'errorEventDefinition');
      if (err) {
        n.type = 'errorEndEvent';
        n.errorCode = err.getAttribute('errorRef') ?? '';
      }
    }
    if (type === 'callActivity') {
      n.subprocessKey = el.getAttribute('calledElement') ?? '';
    }
    if (type === 'exclusiveGateway') {
      const d = el.getAttribute('default');
      if (d) n.defaultEdge = d;
    }
    nodes.push(n);
  };

  for (let i = 0; i < proc.children.length; i++) {
    const el = proc.children[i];
    const type = tagTypes[el.localName ?? ''];
    if (!type) continue;
    parseNode(el, el.localName === 'intermediateCatchEvent' ? 'intermediateTimerEvent' : type);
  }
  for (let i = 0; i < proc.children.length; i++) {
    const el = proc.children[i];
    if ((el.localName ?? '') !== 'sequenceFlow') continue;
    edges.push({
      id: el.getAttribute('id')!,
      source: el.getAttribute('sourceRef')!,
      target: el.getAttribute('targetRef')!,
      name: el.getAttribute('name') ?? undefined,
      isDefault: el.getAttribute('default') === 'true' || undefined,
    });
  }

  // 坐标回填（bpmndi）。
  const diagram = doc.getElementsByTagNameNS('*', 'BPMNShape');
  for (let i = 0; i < diagram.length; i++) {
    const shape = diagram[i];
    const el = shape.getAttribute('bpmnElement');
    const bounds = shape.getElementsByTagNameNS('*', 'Bounds')[0];
    const node = nodes.find((n) => n.id === el);
    if (node && bounds) {
      node.ui = { x: +bounds.getAttribute('x')!, y: +bounds.getAttribute('y')! };
    }
  }

  return { version: 1, key: keyAttr, name: proc.getAttribute('name') ?? undefined, nodes, edges };
}
