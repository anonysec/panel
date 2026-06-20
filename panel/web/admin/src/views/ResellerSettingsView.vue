<script setup lang="ts">
import { useI18n } from '@koris/composables/useI18n'

const { t } = useI18n()
</script>

<template>
  <div class="reseller-settings">
    <div class="empty-state">
      <span class="empty-icon">⚙️</span>
      <h2>{{ t('reseller_settings.title') }}</h2>
      <p>{{ t('reseller_settings.coming_soon') }}</p>
    </div>
  </div>
</template>

<style scoped>
.reseller-settings {
  padding: var(--space-6, 24px);
}

.empty-state {
  text-align: center;
  padding: 80px 20px;
  color: var(--color-muted, #8b98a5);
}

.empty-icon {
  font-size: 48px;
  display: block;
  margin-bottom: 16px;
}

.empty-state h2 {
  font-size: var(--text-xl, 18px);
  font-weight: var(--font-bold, 700);
  color: var(--color-text, #e6edf3);
  margin: 0 0 8px;
}

.empty-state p {
  margin: 0;
  font-size: 14px;
}
</style>
