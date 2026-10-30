# @go-wind/flow-designer-react

**go-wind-flow** 设计器的 React 适配器：`<FlowDesigner>`（BPMN 画布 + 属性面板）
与 `<ApprovalTree>`（垂直审批树，钉钉式业务模式）。

React bindings for [go-wind-flow](https://github.com/tx7do/go-wind-flow) designer.

## Install

```bash
npm i @go-wind/flow-designer-react
```

对等依赖：`react` / `react-dom`（^18 或 ^19）。bpmn-js 样式由宿主引入一次（见下）。

## Usage

```tsx
import { useRef } from 'react';
import { FlowDesigner, type FlowDesignerRef } from '@go-wind/flow-designer-react';

import 'bpmn-js/dist/assets/diagram-js.css';
import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';
import 'bpmn-js/dist/assets/bpmn-js.css';
import '@bpmn-io/properties-panel/dist/assets/properties-panel.css';

export function Editor() {
  const ref = useRef<FlowDesignerRef>(null);
  return (
    <>
      <FlowDesigner
        ref={ref}
        xml={xml}
        theme="auto"      // 'light' | 'dark' | 'auto'
        accent="#00add8"  // 自定义主题色
        height="640px"
      />
      <button onClick={async () => console.log(await ref.current?.getXML())}>保存</button>
    </>
  );
}
```

审批树（业务模式，超出子集时自动回退到画布）：

```tsx
<ApprovalTree document={doc} theme="dark" onChange={(d) => setDoc(d)} />
```

## 主题

`theme`：亮色 / 暗色 / 跟随系统；`accent`：自定义主题色（`--gwf-accent`）。
画布、属性面板、工具栏与审批树共用同一套 token 体系。

## License

[MIT](https://github.com/tx7do/go-wind-flow/blob/main/LICENSE) © Bobo
