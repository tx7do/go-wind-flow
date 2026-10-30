# go-wind-flow 前端设计器（Vue3 / React）

go-wind-flow 流程设计器的前端实现，按 go-wind 家族"多前端并列"惯例组织——
**一个框架无关核心 + Vue3 / React 两个薄适配器**，三包同源、互不依赖：

| 包 | 说明 |
|---|---|
| `@go-wind/flow-designer-core` | 框架无关核心：bpmn-js 建模器封装、go-wind-flow 属性面板（flow:assignees 可视化配置）、designer-json-v1 ↔ BPMN XML 双向转换 |
| `@go-wind/flow-designer-vue` | Vue3 适配器：`<FlowDesigner>` 组件（SFC） |
| `@go-wind/flow-designer-react` | React 适配器：`<FlowDesigner>` 组件（forwardRef + getXML/getDocument） |

## 快速开始（Vue3）

```bash
npm i @go-wind/flow-designer-vue @go-wind/flow-designer-core bpmn-js
```

```vue
<script setup lang="ts">
import { ref } from 'vue';
import { FlowDesigner } from '@go-wind/flow-designer-vue';

const designer = ref();
async function save() {
  const xml = await designer.value.getXML();
  // POST xml → BFF → DeployDefinition(format="bpmn-xml")
}
</script>

<template>
  <FlowDesigner ref="designer" :xml="bpmnXml" height="600px" />
  <button @click="save">发布</button>
</template>
```

## 快速开始（React）

```bash
npm i @go-wind/flow-designer-react @go-wind/flow-designer-core bpmn-js
```

```tsx
import { useRef } from 'react';
import { FlowDesigner, type FlowDesignerRef } from '@go-wind/flow-designer-react';

const designer = useRef<FlowDesignerRef>(null);
async function save() {
  const xml = await designer.current!.getXML();
}

<FlowDesigner ref={designer} xml={bpmnXml} height="600px" />
<button onClick={save}>发布</button>
```

## 属性面板

选中 userTask 后，面板提供 go-wind-flow 审批配置（写入 `flow:assignees`
扩展元素，与后端 `format/bpmn`、`format/designer`、`docs/designer_contract.md`
同一契约）：

- 办理人规则（JSON：USER / STARTER / LEADER / POSITION / ROLE / GROUP / EXPR）
- 审批策略（会签 ALL / 或签 ANY）、驳回模式（终结 / 停留）
- 集合多实例（collection 表达式 + elementVar）
- 任务超时（duration + 超时动作 auto_approve / auto_reject）

## 保存与发布

```
画布 getXML() ──POST──> BFF ──> DeployDefinition(format="bpmn-xml")
                                    └─ 校验 + 不可变版本化（在途实例不受影响）
```

草稿与发布分离、错误回传定位等交互协议见
[docs/designer_contract.md](../docs/designer_contract.md)。

## 开发

```bash
npm install      # 工作区根目录（frontend/）
npm run build    # 依次构建三包
npm run typecheck
```
