import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  base: '/portal/',
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      '@koris/ui': resolve(__dirname, '../shared/components'),
      '@koris/composables': resolve(__dirname, '../shared/composables'),
      '@koris/types': resolve(__dirname, '../shared/types'),
      '@koris/styles': resolve(__dirname, '../shared/styles'),
    }
  },
  build: {
    outDir: 'www',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor': ['vue', 'vue-router', 'pinia'],
        }
      }
    }
  }
})
