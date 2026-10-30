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
export const designerCSS = /* css */ `
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

.gwf-designer[data-gwf-theme='dark'],
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

/* auto 主题跟随系统：dark 系统下应用暗色 tokens 与画布覆盖。 */
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
  .gwf-designer[data-gwf-theme='auto'] .djs-container {
    background: var(--gwf-canvas-bg);
  }
  .gwf-designer[data-gwf-theme='auto'] .djs-visual rect,
  .gwf-designer[data-gwf-theme='auto'] .djs-visual circle,
  .gwf-designer[data-gwf-theme='auto'] .djs-visual ellipse,
  .gwf-designer[data-gwf-theme='auto'] .djs-visual polygon {
    fill: var(--gwf-shape-fill) !important;
  }
  .gwf-designer[data-gwf-theme='auto'] .djs-visual :is(path, rect, circle, polygon, polyline) {
    stroke: var(--gwf-shape-stroke) !important;
  }
  .gwf-designer[data-gwf-theme='auto'] .djs-visual text,
  .gwf-designer[data-gwf-theme='auto'] .djs-label {
    fill: var(--gwf-text) !important;
    stroke: none !important;
  }
}
}

/* ---------- bpmn-js 画布（暗色覆盖 diagram-js 内联视觉） ---------- */
.gwf-designer .djs-container {
  background: var(--gwf-canvas-bg);
}
.gwf-designer[data-gwf-theme='dark'] .djs-visual rect,
.gwf-designer[data-gwf-theme='dark'] .djs-visual circle,
.gwf-designer[data-gwf-theme='dark'] .djs-visual polygon {
  fill: var(--gwf-shape-fill) !important;
}
.gwf-designer[data-gwf-theme='dark'] .djs-visual :is(path, rect, circle, polygon, polyline) {
  stroke: var(--gwf-shape-stroke) !important;
}
.gwf-designer[data-gwf-theme='dark'] .djs-visual text,
.gwf-designer[data-gwf-theme='dark'] .djs-label {
  fill: var(--gwf-text) !important;
  stroke: none !important;
}
.gwf-designer[data-gwf-theme='dark'] .djs-visual path[data-corner] { stroke: none !important; }
.gwf-designer[data-gwf-theme='dark'] .djs-palette,
.gwf-designer[data-gwf-theme='dark'] .djs-context-pad {
  background: var(--gwf-node-bg);
  border-color: var(--gwf-border);
}
.gwf-designer[data-gwf-theme='dark'] .djs-palette .entry,
.gwf-designer[data-gwf-theme='dark'] .djs-context-pad .entry {
  background-color: var(--gwf-shape-stroke);
}
.gwf-designer[data-gwf-theme='dark'] .djs-palette .separator {
  border-color: var(--gwf-border);
}
.gwf-designer[data-gwf-theme='dark'] .djs-tooltip,
.gwf-designer[data-gwf-theme='dark'] .djs-popup {
  background: var(--gwf-node-bg);
  color: var(--gwf-text);
  border: 1px solid var(--gwf-border);
}
.gwf-designer[data-gwf-theme='dark'] .djs-bendpoint,
.gwf-designer[data-gwf-theme='dark'] .djs-segment-dragger .handle {
  fill: var(--gwf-accent);
  stroke: var(--gwf-accent);
}

/* ---------- properties-panel（上游 CSS 变量映射） ---------- */
.gwf-designer .bio-properties-panel {
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
  font-size: 14px;
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
