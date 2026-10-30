# @go-wind/flow-designer-vue

**go-wind-flow** 设计器的 Vue 3 适配器：`<FlowDesigner>`（BPMN 画布 + 属性面板）
与 `<ApprovalTree>`（垂直审批树，钉钉式业务模式）。

Vue 3 bindings for [go-wind-flow](https://github.com/tx7do/go-wind-flow) designer.

## Install

```bash
npm i @go-wind/flow-designer-vue
```

对等依赖：`vue ^3.4`。bpmn-js 样式由宿主引入一次（见下）。

## Usage

```vue
<script setup lang="ts">
import { FlowDesigner, ApprovalTree } from '@go-wind/flow-designer-vue';
import type { DesignerHandle } from '@go-wind/flow-designer-vue';

import 'bpmn-js/dist/assets/diagram-js.css';
import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';
import 'bpmn-js/dist/assets/bpmn-js.css';
import '@bpmn-io/properties-panel/dist/assets/properties-panel.css';

let handle: DesignerHandle | null = null;
async function save() {
  const xml = await handle?.getXML();   // → BFF → Deploy(format=bpmn-xml)
}
</script>

<template>
  <FlowDesigner
    xml="<definitions …/>"
    theme="auto"
    accent="#00add8"
    height="640px"
    @ready="(h) => (handle = h)"
  />
</template>
```

审批树（业务模式，超出子集时自动回退到画布）：

```vue
<ApprovalTree :document="doc" theme="dark" @change="(d) => (doc = d)" />
```

## 主题

`theme`：`'light' | 'dark' | 'auto'`（缺省跟随系统）；`accent` 自定义主题色
（`--gwf-accent`，弱色/高亮经 `color-mix` 自动派生）。画布、属性面板、
工具栏与审批树共用同一套 token。

## License

[MIT](https://github.com/tx7do/go-wind-flow/blob/main/LICENSE) © Bobo
