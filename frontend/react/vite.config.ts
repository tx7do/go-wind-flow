import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: 'src/index.ts',
      name: 'GoWindFlowDesignerReact',
      formats: ['es'],
      fileName: () => 'index.js',
    },
    rollupOptions: {
      external: ['react', 'react-dom', '@go-wind/flow-designer-core', 'bpmn-js', 'bpmn-js/lib/Modeler', 'bpmn-js-properties-panel', '@bpmn-io/properties-panel'],
    },
    sourcemap: true,
    emptyOutDir: true,
  },
});
