/**
 * 主题体系：design tokens（CSS 变量）+ 亮色 / 暗色 / 跟随系统 三态。
 *
 * 用法：createDesigner({ theme: 'dark' }) 或 setTheme('dark')——
 * 在组件根元素上设置 data-gwf-theme，全部样式（含 bpmn-js 画布与
 * properties-panel）走下方 token，宿主无需自行适配。
 */

export type Theme = 'light' | 'dark' | 'auto';

export const THEME_ATTR = 'data-gwf-theme';

/** 确保组件样式表已注入（幂等；树/画布组件挂载时调用）。 */
export function ensureStyles(): void {
  if (typeof document === "undefined") return;
  if (document.getElementById("gwf-designer-styles")) return;
  const style = document.createElement("style");
  style.id = "gwf-designer-styles";
  style.textContent = designerCSS;
  document.head.appendChild(style);
}

/** 应用主题到组件根元素（auto = 跟随系统 prefers-color-scheme）。 */
export function applyTheme(root: HTMLElement, theme: Theme): void {
  root.setAttribute(THEME_ATTR, theme);
}

export function applyThemeToAll(theme: Theme): void {
  document.querySelectorAll(`[${THEME_ATTR}]`).forEach((el) => {
    (el as HTMLElement).setAttribute(THEME_ATTR, theme);
  });
}

/** 自定义主题色：注入 --gwf-accent，弱色/高亮经 color-mix 自动派生。 */
export function applyAccent(root: HTMLElement, accent: string): void {
  root.style.setProperty('--gwf-accent', accent);
}

/**
 * 组件样式表（含亮/暗 token、bpmn-js 画布暗色覆盖、properties-panel 变量映射）。
 * createDesigner 会自动注入一次；宿主也可自行引入 `styles.css` 并通过
 * THEME_ATTR 手动控主题。
 */
// 显式标注为 string：否则 TS 会把整段 CSS 推断成字面量类型，声明文件膨胀到 20KB。
export const designerCSS: string = /* css */ `
.gwf-designer {
  --gwf-bg: #ffffff;
  --gwf-canvas-bg: #f7f8fa;
  --gwf-border: #dcdfe6;
  --gwf-border-strong: #c0c4cc;
  --gwf-text: #303133;
  --gwf-text-secondary: #909399;
  --gwf-accent: #00add8;
  --gwf-accent-weak: color-mix(in srgb, var(--gwf-accent) 12%, transparent);
  --gwf-node-bg: #ffffff;
  --gwf-node-border: #b1b7bd;
  --gwf-shape-fill: #ffffff;
  --gwf-shape-stroke: #34404d;
  --gwf-edge: #34404d;
  --gwf-danger: #f56c6c;
  --gwf-shadow: 0 1px 4px rgba(0, 0, 0, 0.12);

  background: var(--gwf-bg);
  color: var(--gwf-text);
  font-family: system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
  font-size: 14px;
}

.gwf-designer[data-gwf-theme='dark'] {
  --gwf-bg: #14161a;
  --gwf-canvas-bg: #191c21;
  --gwf-border: #2c313a;
  --gwf-border-strong: #3d434e;
  --gwf-text: #e5e7eb;
  --gwf-text-secondary: #9ca3af;
  --gwf-accent: #29c1e8;
  --gwf-accent-weak: color-mix(in srgb, var(--gwf-accent) 14%, transparent);
  --gwf-node-bg: #1e2228;
  --gwf-node-border: #4b5563;
  --gwf-shape-fill: #23272f;
  --gwf-shape-stroke: #9be2f5;
  --gwf-edge: #9be2f5;
  --gwf-danger: #f87171;
  --gwf-shadow: 0 1px 4px rgba(0, 0, 0, 0.5);
}

/* auto 主题跟随系统：仅当系统为暗色时切换 token，亮色系统保持上面的一套。 */
@media (prefers-color-scheme: dark) {
  .gwf-designer[data-gwf-theme='auto'] {
    --gwf-bg: #14161a;
    --gwf-canvas-bg: #191c21;
    --gwf-border: #2c313a;
    --gwf-border-strong: #3d434e;
    --gwf-text: #e5e7eb;
    --gwf-text-secondary: #9ca3af;
    --gwf-accent: #29c1e8;
    --gwf-accent-weak: color-mix(in srgb, var(--gwf-accent) 14%, transparent);
    --gwf-node-bg: #1e2228;
    --gwf-node-border: #4b5563;
    --gwf-shape-fill: #23272f;
    --gwf-shape-stroke: #9be2f5;
    --gwf-edge: #9be2f5;
    --gwf-danger: #f87171;
    --gwf-shadow: 0 1px 4px rgba(0, 0, 0, 0.5);
  }
}

/* ---------- bpmn-js 画布 ----------
 * 以下覆盖只依赖 token、不区分主题：token 随 light/dark/auto 切换，
 * 避免「auto 有暗色 token 却没有暗色覆盖规则」这类半暗半亮状态。 */
/* diagram-js 自身主题变量映射。必须挂在 .djs-container（即 .djs-parent，
 * diagram-js 在此声明默认值）上——挂在 .gwf-designer 会被内层声明压住。 */
.gwf-designer .djs-container {
  --palette-background-color: var(--gwf-node-bg);
  --palette-border-color: var(--gwf-border);
  --palette-entry-color: var(--gwf-text);
  --palette-entry-hover-color: var(--gwf-accent);
  --palette-entry-selected-color: var(--gwf-accent);
  --palette-separator-color: var(--gwf-border);
  --palette-toggle-hover-background-color: var(--gwf-accent-weak);
  --context-pad-entry-background-color: var(--gwf-node-bg);
  --context-pad-entry-hover-background-color: var(--gwf-accent-weak);
  --popup-background-color: var(--gwf-node-bg);
  --popup-border-color: var(--gwf-border);
  --bendpoint-fill-color: var(--gwf-accent);
  --tooltip-error-background-color: color-mix(in srgb, var(--gwf-danger) 16%, var(--gwf-node-bg));
  --tooltip-error-border-color: var(--gwf-danger);
  --tooltip-error-color: var(--gwf-danger);

  background: var(--gwf-canvas-bg);
}
.gwf-designer .djs-visual rect,
.gwf-designer .djs-visual circle,
.gwf-designer .djs-visual ellipse,
.gwf-designer .djs-visual polygon {
  fill: var(--gwf-shape-fill) !important;
}
.gwf-designer .djs-visual :is(path, rect, circle, ellipse, polygon, polyline) {
  stroke: var(--gwf-shape-stroke) !important;
}
.gwf-designer .djs-visual text,
.gwf-designer .djs-label {
  fill: var(--gwf-text) !important;
  stroke: none !important;
}
.gwf-designer .djs-visual path[data-corner] { stroke: none !important; }

/* palette / context-pad 的底色与描边由上面的 diagram-js 变量映射统一驱动，
 * 这里只管尺寸与交互（容器背景尤其不要画：context-pad 容器比条目宽，
 * 上色会露出一条空槽）。 */
.gwf-designer .djs-palette .entry {
  width: 46px;
  height: 46px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
}
.gwf-designer .djs-palette .entry:hover {
  background: var(--gwf-accent-weak);
}
/* context pad：diagram-js 把容器固定为 72px 宽、条目 22px。条目须保持行内
 * 排布（3 列 × 24px 正好铺满），改成块级会在右侧留下空槽。 */
.gwf-designer .djs-context-pad .entry {
  box-sizing: border-box;
  width: 22px;
  height: 22px;
  display: inline-block;
  text-align: center;
  line-height: 20px;
  font-size: 13px;
  border: 1px solid var(--gwf-border);
  border-radius: 4px;
  box-shadow: var(--gwf-shadow);
}
.gwf-designer .djs-context-pad .entry:hover {
  border-color: var(--gwf-accent);
}
.gwf-designer .djs-tooltip,
.gwf-designer .djs-popup {
  border: 1px solid var(--gwf-border);
}
.gwf-designer .djs-bendpoint,
.gwf-designer .djs-segment-dragger .handle {
  fill: var(--gwf-accent);
  stroke: var(--gwf-accent);
}

/* ---------- diagram-js 悬停/选中/折点（显式声明，两主题均走 token） ---------- */
.gwf-designer .djs-outline {
  visibility: hidden;
}
.gwf-designer .djs-element.hover .djs-outline {
  visibility: visible;
  stroke: var(--gwf-accent);
  stroke-width: 1.5px;
  stroke-dasharray: 4 3;
}
.gwf-designer .djs-element.selected .djs-outline {
  visibility: visible;
  stroke: var(--gwf-accent);
  stroke-width: 1.5px;
}
.gwf-designer .djs-bendpoints .djs-bendpoint {
  display: none;
}
.gwf-designer .djs-bendpoints.hovering .djs-bendpoint,
.gwf-designer .djs-bendpoints.selected .djs-bendpoint {
  display: block;
}
.gwf-designer .djs-bendpoints circle {
  fill: var(--gwf-node-bg);
  stroke: var(--gwf-accent);
  stroke-width: 2px;
}
.gwf-designer .djs-segment-dragger .djs-dragger {
  fill: var(--gwf-accent);
}

/* properties-panel：新版（@bpmn-io/theme）在 .bio-theme-parent 声明 --bio-* 语义
 * token，旧版用 --color-grey-*；两套都映射到 gwf token，版本升级不用改样式。 */
.gwf-designer .bio-theme-parent,
.gwf-designer .bio-properties-panel {
  --bio-surface: var(--gwf-node-bg);
  --bio-surface-overlay: var(--gwf-node-bg);
  --bio-surface-subtle: var(--gwf-bg);
  --bio-surface-medium: var(--gwf-bg);
  --bio-surface-strong: var(--gwf-border);
  --bio-surface-inverted: var(--gwf-text);
  --bio-text: var(--gwf-text);
  --bio-text-subtle: var(--gwf-text-secondary);
  --bio-text-subtlest: var(--gwf-text-secondary);
  --bio-text-on-primary: #ffffff;
  --bio-text-on-inverted: var(--gwf-bg);
  --bio-border: var(--gwf-border);
  --bio-border-subtle: var(--gwf-border);
  --bio-border-disabled: var(--gwf-border);
  --bio-border-input: var(--gwf-border-strong);
  --bio-primary: var(--gwf-accent);
  --bio-primary-hover: var(--gwf-accent);
  --bio-primary-surface: var(--gwf-accent-weak);
  --bio-primary-surface-strong: var(--gwf-accent-weak);
  --bio-neutral: var(--gwf-text-secondary);
  --bio-danger: var(--gwf-danger);
  --bio-danger-surface: color-mix(in srgb, var(--gwf-danger) 16%, transparent);
  --bio-danger-surface-subtle: var(--gwf-danger-surface);
  --bio-info: var(--gwf-accent);
  --bio-focus: var(--gwf-accent);
  --bio-focus-ring: color-mix(in srgb, var(--gwf-accent) 35%, transparent);

  --color-grey-225-10-15: var(--gwf-text);
  --color-grey-225-10-35: var(--gwf-text-secondary);
  --color-grey-225-10-55: var(--gwf-text-secondary);
  --color-grey-225-10-75: var(--gwf-border-strong);
  --color-grey-225-10-80: var(--gwf-border);
  --color-grey-225-10-85: var(--gwf-border);
  --color-grey-225-10-90: var(--gwf-border);
  --color-grey-225-10-95: var(--gwf-bg);
  --color-blue-205-100-45: var(--gwf-accent);
  --color-blue-205-100-50: var(--gwf-accent);
  --color-blue-205-100-80: var(--gwf-accent-weak);
  --color-red-360-100-45: var(--gwf-danger);
  --color-white: var(--gwf-node-bg);
  --text-base-color: var(--gwf-text);
}
.gwf-designer .bio-properties-panel {
  background: var(--gwf-node-bg);
  color: var(--gwf-text);
}
.gwf-designer .bio-properties-panel input,
.gwf-designer .bio-properties-panel textarea,
.gwf-designer .bio-properties-panel select {
  background: var(--gwf-bg);
  color: var(--gwf-text);
  border-color: var(--gwf-border-strong);
}
.gwf-designer .bio-properties-panel-group-header {
  background: var(--gwf-bg);
}

/* ---------- 垂直审批树 v2（卡片化 + 分支列 + 侧边编辑器） ---------- */
.gwf-tree-area {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px 24px 32px;
}
.gwf-tree-wrap {
  display: flex;
  align-items: stretch;
}
.gwf-tree-area {
  flex: 1;
  min-width: 0;
}
.gwf-branch-col .gwf-v2-node {
  width: 100%;
  box-sizing: border-box;
}
.gwf-editor {
  flex-shrink: 0;
}
.gwf-tree-area .gwf-start-node,
.gwf-tree-area .gwf-end-node {
  width: 120px;
  text-align: center;
  padding: 8px 0;
  border-radius: 8px;
  background: var(--gwf-accent-weak);
  color: var(--gwf-accent);
  font-weight: 600;
  font-size: 13px;
}
.gwf-v2-node {
  width: 248px;
  position: relative;
  background: var(--gwf-node-bg);
  border: 1px solid var(--gwf-node-border);
  border-radius: 10px;
  padding: 10px 12px 10px 48px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s, transform 0.15s;
}
.gwf-v2-node:hover {
  border-color: var(--gwf-accent);
  transform: translateY(-1px);
  box-shadow: 0 4px 14px var(--gwf-accent-weak);
}
.gwf-v2-node.selected {
  border-color: var(--gwf-accent);
  box-shadow: 0 0 0 2px var(--gwf-accent-weak);
}
.gwf-v2-avatar {
  position: absolute;
  left: 11px;
  top: 50%;
  transform: translateY(-50%);
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--gwf-accent-weak);
  color: var(--gwf-accent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
}
.gwf-v2-name { font-weight: 600; font-size: 13px; }
.gwf-v2-meta { margin-top: 2px; font-size: 12px; color: var(--gwf-text-secondary); }
.gwf-v2-actions {
  position: absolute;
  top: 8px;
  right: 8px;
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.15s;
}
.gwf-v2-node:hover .gwf-v2-actions,
.gwf-v2-node.selected .gwf-v2-actions { opacity: 1; }
.gwf-connector {
  position: relative;
  width: 248px;
  height: 32px;
}
.gwf-connector::before {
  content: "";
  position: absolute;
  left: 50%;
  top: 0;
  bottom: 0;
  width: 2px;
  margin-left: -1px;
  background: var(--gwf-node-border);
}
.gwf-connector .gwf-add {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--gwf-node-bg);
  border: 1px solid var(--gwf-accent);
  color: var(--gwf-accent);
  font-size: 15px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  line-height: 1;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s, background 0.15s, color 0.15s;
  z-index: 1;
}
.gwf-connector:hover .gwf-add { opacity: 1; }
.gwf-connector .gwf-add:hover {
  background: var(--gwf-accent);
  color: #fff;
}
.gwf-branch-cols {
  display: flex;
  gap: 14px;
  align-items: stretch;
}
.gwf-branch-col {
  flex: 1;
  min-width: 220px;
  border: 1px solid var(--gwf-node-border);
  border-radius: 12px;
  padding: 8px;
  background: var(--gwf-bg);
  display: flex;
  flex-direction: column;
  align-items: center;
}
.gwf-branch-tag {
  display: flex;
  gap: 6px;
  align-items: center;
  width: 100%;
  margin-bottom: 4px;
}
.gwf-branch-tag .gwf-badge {
  background: var(--gwf-accent-weak);
  color: var(--gwf-accent);
  border-radius: 4px;
  padding: 2px 8px;
  font-size: 12px;
  white-space: nowrap;
}
.gwf-branch-tag input {
  flex: 1;
  min-width: 0;
  background: var(--gwf-bg);
  color: var(--gwf-text);
  border: 1px solid var(--gwf-border);
  border-radius: 4px;
  padding: 2px 6px;
  font-size: 12px;
}
.gwf-editor {
  width: 320px;
  border-left: 1px solid var(--gwf-border);
  padding: 16px;
  background: var(--gwf-node-bg);
  overflow-y: auto;
}
.gwf-editor h4 {
  margin: 0 0 12px;
  font-size: 14px;
}
.gwf-editor label {
  display: block;
  font-size: 12px;
  color: var(--gwf-text-secondary);
  margin: 10px 0 4px;
}
.gwf-editor input,
.gwf-editor select {
  width: 100%;
  box-sizing: border-box;
  background: var(--gwf-bg);
  color: var(--gwf-text);
  border: 1px solid var(--gwf-border);
  border-radius: 6px;
  padding: 6px 8px;
  font-size: 13px;
}
.gwf-editor .gwf-row {
  display: flex;
  gap: 6px;
  margin-bottom: 6px;
  align-items: center;
}
.gwf-editor .gwf-row input,
.gwf-editor .gwf-row select {
  flex: 1;
  min-width: 0;
}
.gwf-editor .gwf-btn {
  border: none;
  background: var(--gwf-accent-weak);
  color: var(--gwf-accent);
  border-radius: 4px;
  padding: 3px 10px;
  cursor: pointer;
  font-size: 12px;
}
.gwf-editor .gwf-btn.danger {
  background: transparent;
  color: var(--gwf-danger);
}
.gwf-v2-fade-enter-active { transition: all 0.2s ease-out; }

/* ---------- 垂直审批树（DOM 组件） ---------- */
.gwf-tree {
  overflow-y: auto;
  padding: 16px;
  background: var(--gwf-canvas-bg);
}
.gwf-tree-node {
  position: relative;
  width: 260px;
  margin: 0 auto;
  background: var(--gwf-node-bg);
  border: 1px solid var(--gwf-node-border);
  border-radius: 8px;
  padding: 10px 12px;
  box-shadow: var(--gwf-shadow);
}
.gwf-tree-node.selected {
  border-color: var(--gwf-accent);
  box-shadow: 0 0 0 2px var(--gwf-accent-weak);
}
.gwf-tree-node .gwf-node-name {
  font-weight: 600;
}
.gwf-tree-node .gwf-node-meta {
  margin-top: 4px;
  color: var(--gwf-text-secondary);
  font-size: 12px;
}
.gwf-tree-node .gwf-node-actions {
  position: absolute;
  top: 6px;
  right: 8px;
  display: flex;
  gap: 6px;
  opacity: 0;
  transition: opacity 0.15s;
}
.gwf-tree-node:hover .gwf-node-actions,
.gwf-tree-node.selected .gwf-node-actions {
  opacity: 1;
}
.gwf-tree-node .gwf-btn {
  border: none;
  background: var(--gwf-accent-weak);
  color: var(--gwf-accent);
  border-radius: 4px;
  padding: 1px 7px;
  cursor: pointer;
  font-size: 12px;
}
.gwf-tree-node .gwf-btn.danger {
  background: rgba(246, 108, 108, 0.14);
  color: var(--gwf-danger);
}
.gwf-tree-connector {
  width: 2px;
  height: 24px;
  margin: 0 auto;
  background: var(--gwf-border-strong);
}
.gwf-tree-add {
  width: 260px;
  margin: 0 auto 8px;
  text-align: center;
}
.gwf-tree-add .gwf-btn {
  width: 100%;
  border: 1px dashed var(--gwf-border-strong);
  background: transparent;
  color: var(--gwf-text-secondary);
  border-radius: 6px;
  padding: 6px 0;
  cursor: pointer;
}
.gwf-tree-add .gwf-btn:hover {
  color: var(--gwf-accent);
  border-color: var(--gwf-accent);
}
.gwf-tree-branches {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.gwf-tree-branch {
  flex: 1;
  min-width: 0;
  border: 1px dashed var(--gwf-border-strong);
  border-radius: 8px;
  padding: 8px;
}
.gwf-tree-branch .gwf-branch-head {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}
.gwf-tree-branch .gwf-branch-head input {
  flex: 1;
  min-width: 0;
  background: var(--gwf-bg);
  color: var(--gwf-text);
  border: 1px solid var(--gwf-border);
  border-radius: 4px;
  padding: 2px 6px;
  font-size: 12px;
}
.gwf-tree-branch .gwf-branch-head .gwf-badge {
  background: var(--gwf-accent-weak);
  color: var(--gwf-accent);
  border-radius: 4px;
  padding: 2px 6px;
  font-size: 12px;
  white-space: nowrap;
}

/* ---------- 配置表单 ---------- */
.gwf-form {
  border-top: 1px solid var(--gwf-border);
  margin-top: 8px;
  padding-top: 8px;
  display: grid;
  gap: 6px;
}
.gwf-form label {
  display: grid;
  gap: 2px;
  font-size: 12px;
  color: var(--gwf-text-secondary);
}
.gwf-form input,
.gwf-form select {
  background: var(--gwf-bg);
  color: var(--gwf-text);
  border: 1px solid var(--gwf-border);
  border-radius: 4px;
  padding: 4px 6px;
  font-size: 13px;
}
.gwf-form .gwf-row {
  display: flex;
  gap: 6px;
  align-items: center;
}
.gwf-form .gwf-row input,
.gwf-form .gwf-row select {
  flex: 1;
  min-width: 0;
}
.gwf-designer .gwf-btn {
  border: none;
  background: var(--gwf-accent-weak);
  color: var(--gwf-accent);
  border-radius: 4px;
  padding: 3px 10px;
  cursor: pointer;
  font-size: 12px;
}
.gwf-designer .gwf-btn.danger {
  background: transparent;
  color: var(--gwf-danger);
}
.gwf-editor-inline {
  border-top: 1px solid var(--gwf-border);
  margin-top: 8px;
  padding-top: 8px;
  display: grid;
  gap: 6px;
  width: 100%;
}
.gwf-editor-inline label {
  font-size: 12px;
  color: var(--gwf-text-secondary);
}
.gwf-editor-inline input,
.gwf-editor-inline select {
  width: 100%;
  box-sizing: border-box;
  background: var(--gwf-bg);
  color: var(--gwf-text);
  border: 1px solid var(--gwf-border);
  border-radius: 4px;
  padding: 4px 6px;
  font-size: 12px;
}
.gwf-editor-inline .gwf-row {
  display: flex;
  gap: 4px;
}
.gwf-editor-inline .gwf-row input,
.gwf-editor-inline .gwf-row select {
  flex: 1;
  min-width: 0;
}
.gwf-designer .gwf-btn {
  border: none;
  background: var(--gwf-accent-weak);
  color: var(--gwf-accent);
  border-radius: 4px;
  padding: 3px 10px;
  cursor: pointer;
  font-size: 12px;
}
.gwf-designer .gwf-btn.danger {
  background: transparent;
  color: var(--gwf-danger);
}
.gwf-editor-inline {
  border-top: 1px solid var(--gwf-border);
  margin-top: 8px;
  padding-top: 8px;
  display: grid;
  gap: 6px;
  width: 100%;
}
.gwf-editor-inline label {
  font-size: 12px;
  color: var(--gwf-text-secondary);
}
.gwf-editor-inline input,
.gwf-editor-inline select {
  width: 100%;
  box-sizing: border-box;
  background: var(--gwf-bg);
  color: var(--gwf-text);
  border: 1px solid var(--gwf-border);
  border-radius: 4px;
  padding: 4px 6px;
  font-size: 12px;
}
.gwf-editor-inline .gwf-row {
  display: flex;
  gap: 4px;
}
.gwf-editor-inline .gwf-row input,
.gwf-editor-inline .gwf-row select {
  flex: 1;
  min-width: 0;
}
`;
