# @go-wind/flow-designer-core

**go-wind-flow** 的流程设计器核心（框架无关）：基于 bpmn-js 的 BPMN 建模器 +
go-wind-flow 属性面板扩展 + `designer-json-v1` 双向序列化。

Framework-agnostic designer core for [go-wind-flow](https://github.com/tx7do/go-wind-flow):
bpmn-js modeler with the `flow:assignees` properties extension and
`designer-json-v1` (de)serialization.

## Install

```bash
npm i @go-wind/flow-designer-core bpmn-js
```

bpmn-js 的三份样式与属性面板样式由宿主引入（设计器依赖它们渲染）：

```ts
import 'bpmn-js/dist/assets/diagram-js.css';
import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';
import 'bpmn-js/dist/assets/bpmn-js.css';
import '@bpmn-io/properties-panel/dist/assets/properties-panel.css';
```

## Usage

```ts
import { createDesigner } from '@go-wind/flow-designer-core';

const designer = createDesigner({
  canvas: '#canvas',
  panel: '#panel',
  theme: 'auto',      // 'light' | 'dark' | 'auto'
  accent: '#00add8',  // --gwf-accent，弱色/高亮自动派生
});

await designer.loadXML(bpmnXml);
const xml = await designer.getXML();        // 保存 → BFF → Deploy(format=bpmn-xml)
const doc = await designer.getDocument();   // designer-json-v1
designer.setTheme('dark');
designer.destroy();
```

## 内容

- `createDesigner` — 建模器 + 属性面板 + 主题（亮/暗/跟随系统、自定义主题色）
- `documentToXML` / `xmlToDocument` — BPMN XML ↔ `designer-json-v1`
- `documentToTree` / `treeToDocument` + 树操作（纯函数，返回新树）— 垂直审批树数据层
- `applyTheme` / `applyAccent` / `designerCSS` — 独立复用主题体系

Vue3 / React 适配器：`@go-wind/flow-designer-vue`、`@go-wind/flow-designer-react`。

## License

[MIT](https://github.com/tx7do/go-wind-flow/blob/main/LICENSE) © Bobo
