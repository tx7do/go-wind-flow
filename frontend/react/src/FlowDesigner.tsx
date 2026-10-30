import { useEffect, useImperativeHandle, useRef, forwardRef } from 'react';
import { createDesigner, type DesignerDocument, type DesignerHandle } from '@go-wind/flow-designer-core';

export interface FlowDesignerProps {
  /** BPMN XML 或 designer-json-v1 文档（自动嗅探：'<' 开头为 XML）。 */
  xml?: string;
  document?: DesignerDocument;
  /** 画布高度（CSS 值，缺省 600px）。 */
  height?: string;
  onReady?: (handle: DesignerHandle) => void;
  onError?: (err: unknown) => void;
}

export interface FlowDesignerRef {
  getXML(): Promise<string>;
  getDocument(key?: string): Promise<DesignerDocument>;
}

export const FlowDesigner = forwardRef<FlowDesignerRef, FlowDesignerProps>(function FlowDesigner(
  { xml, document, height, onReady, onError },
  ref,
) {
  const canvasRef = useRef<HTMLDivElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const handleRef = useRef<DesignerHandle | null>(null);

  useImperativeHandle(ref, () => ({
    async getXML() {
      return handleRef.current ? handleRef.current.getXML() : '';
    },
    async getDocument(key?: string) {
      if (!handleRef.current) throw new Error('designer not mounted');
      return handleRef.current.getDocument(key);
    },
  }));

  useEffect(() => {
    if (!canvasRef.current || !panelRef.current) return;
    const handle = createDesigner({ canvas: canvasRef.current, panel: panelRef.current });
    handleRef.current = handle;
    onReady?.(handle);
    return () => {
      handle.destroy();
      handleRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const handle = handleRef.current;
    if (!xml && !document) return;
    const load = async () => {
      if (!handle) return;
      try {
        if (xml) {
          await handle.loadXML(xml);
        } else if (document) {
          await handle.loadDocument(document);
        }
        handle.zoom('fit-viewport');
      } catch (err) {
        onError?.(err);
      }
    };
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [xml, document]);

  return (
    <div className="gwf-designer" style={{ display: 'flex', gap: 8 }}>
      <div ref={canvasRef} className="gwf-canvas" style={{ flex: 1, border: '1px solid #dcdfe6', height: height ?? '600px' }} />
      <div ref={panelRef} className="gwf-panel" style={{ width: 320, border: '1px solid #dcdfe6', overflowY: 'auto' }} />
    </div>
  );
});
