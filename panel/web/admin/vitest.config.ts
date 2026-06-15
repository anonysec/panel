import { defineConfig } from 'vitest/config'
import { resolve } from 'path'

export default defineConfig({
  test: {
    environment: 'happy-dom',
    globals: true,
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
  resolve: {
    alias: {
      '@': resolve(__dirname, './src'),
      '@koris/types': resolve(__dirname, '../shared/types'),
      '@koris/types/components': resolve(__dirname, '../shared/types/components'),
      '@koris/composables': resolve(__dirname, '../shared/composables'),
      '@koris/ui': resolve(__dirname, '../shared/components'),
      '@koris/styles': resolve(__dirname, '../shared/styles'),
      'vue': resolve(__dirname, '../shared/node_modules/vue'),
    },
  },
})
