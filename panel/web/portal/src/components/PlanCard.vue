<script setup lang="ts">
import { computed } from 'vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

interface Props {
  planName: string
  status: string
  expiresAt?: string
}

const props = defineProps<Props>()

const formattedExpiry = computed(() => {
  if (!props.expiresAt) return 'No expiry set'
  return new Intl.DateTimeFormat('en', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
  }).format(new Date(props.expiresAt))
})

const statusVariant = computed(() => {
  switch (props.status) {
    case 'active': return 'active'
    case 'expired': return 'expired'
    case 'disabled': return 'disabled'
    default: return 'expired'
  }
})
</script>
<template>
  <div class="plan-card">
    <div class="plan-card__header">
      <span class="plan-card__label">Current Plan</span>
      <KStatusPill :status="statusVariant">{{ status }}</KStatusPill>
    </div>
    <h3 class="plan-card__name">{{ planName }}</h3>
    <div class="plan-card__expiry">
      <span class="plan-card__expiry-label">Expires:</span>
      <span class="plan-card__expiry-value">{{ formattedExpiry }}</span>
    </div>
  </div>
</template>
<style scoped>
.plan-card {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.plan-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-3);
}
.plan-card__label {
  font-size: var(--text-xs);
  color: var(--color-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.plan-card__name {
  font-size: var(--text-xl);
  font-weight: 700;
  margin-bottom: var(--space-2);
}
.plan-card__expiry {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.plan-card__expiry-label {
  margin-right: var(--space-1);
}
</style>
