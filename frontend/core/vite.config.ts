import { defineConfig } from 'vite';

export default defineConfig({
  build: {
    lib: {
      entry: 'src/index.ts',
      name: 'GoWindFlowDesignerCore',
      formats: ['es'],
      fileName: () => 'index.js',
    },
    rollupOptions: {
      external: ['bpmn-js', 'bpmn-js/lib/Modeler', 'bpmn-js-properties-panel', '@bpmn-io/properties-panel', 'min-dash'],
    },
    sourcemap: true,
    emptyOutDir: true,
  },
});
