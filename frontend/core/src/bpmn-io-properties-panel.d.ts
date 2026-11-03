// @bpmn-io/properties-panel 的最小类型面：面板条目工厂（v5 面板只渲染
// 带 component 的条目对象，须用工厂构建）。仅声明本库实际用到的 API。
declare module '@bpmn-io/properties-panel' {
  export function TextFieldEntry(props: Record<string, unknown>): unknown;
  export function SelectEntry(props: Record<string, unknown>): unknown;
}
