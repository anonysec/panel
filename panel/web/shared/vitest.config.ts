import { defineConfig } from 'vitest/config'
import { resolve } from 'path'

export default defineConfig({
  test: {
    environment: 'happy-dom',
    globals: true,
    include: ['**/*.{test,spec}.{ts,tsx}'],
  },
  resolve: {
    alias: {
      '@koris/types': resolve(__dirname, './types'),
      '@koris/types/components': resolve(__dirname, './types/components'),
      '@koris/composables': resolve(__dirname, './composables'),
      '@koris/composables/useFormValidation': resolve(__dirname, './composables/useFormValidation'),
      '@koris/composables/useWebSocket': resolve(__dirname, './composables/useWebSocket'),
      '@koris/composables/useApi': resolve(__dirname, './composables/useApi'),
      'vue': resolve(__dirname, './node_modules/vue'),
    },
  },
})
