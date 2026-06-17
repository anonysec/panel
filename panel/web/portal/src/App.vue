<script setup lang="ts">
import { onMounted } from 'vue'
import { useApi } from '@koris/composables/useApi'
import { useTheme, availableThemes } from '@koris/composables/useTheme'
import type { ThemeMode, UITheme } from '@koris/composables/useTheme'

const { get } = useApi()
const { setMode, setTheme } = useTheme()

onMounted(async () => {
  try {
    const res = await get<{ ok: boolean; settings: Record<string, string> }>('/api/panel-settings')
    if (res.settings) {
      if (res.settings.ui_theme && availableThemes.some((t) => t.id === res.settings.ui_theme)) {
        setTheme(res.settings.ui_theme as UITheme)
      }
      if (res.settings.ui_mode && ['system', 'dark', 'light'].includes(res.settings.ui_mode)) {
        setMode(res.settings.ui_mode as ThemeMode)
      }
    }
  } catch {
    // Use localStorage defaults on error
  }
})
</script>

<template><router-view /></template>
