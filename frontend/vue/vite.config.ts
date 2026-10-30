import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
  plugins: [vue()],
  build: {
    lib: {
      entry: 'src/index.ts',
      name: 'GoWindFlowDesignerVue',
      formats: ['es'],
      fileName: () => 'index.js',
    },
    rollupOptions: {
      external: ['vue', '@go-wind/flow-designer-core', 'bpmn-js', 'bpmn-js/lib/Modeler', 'bpmn-js-properties-panel', '@bpmn-io/properties-panel'],
    },
    sourcemap: true,
    emptyOutDir: true,
  },
});
