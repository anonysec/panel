<template>
  <span
    :class="[
      'k-status-pill',
      `k-status-pill--${computedVariant}`,
      `k-status-pill--${size}`,
    ]"
    role="status"
    :aria-label="`Status: ${status}`"
  >
    <span class="k-status-pill__dot" aria-hidden="true" />
    <span class="k-status-pill__text">{{ status }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

export interface KStatusPillProps {
  status: string
  size?: 'sm' | 'md'
}

const props = withDefaults(defineProps<KStatusPillProps>(), {
  size: 'md',
})

const okStatuses = ['active', 'running', 'online', 'open', 'approved', 'completed']
const warnStatuses = ['limited', 'pending', 'stale']
const badStatuses = ['expired', 'failed', 'rejected', 'cancelled']
const idleStatuses = ['disabled', 'closed', 'offline']

const computedVariant = computed<'ok' | 'warn' | 'bad' | 'idle'>(() => {
  const s = props.status.toLowerCase()
  if (okStatuses.includes(s)) return 'ok'
  if (warnStatuses.includes(s)) return 'warn'
  if (badStatuses.includes(s)) return 'bad'
  if (idleStatuses.includes(s)) return 'idle'
  return 'idle'
})
</script>

<style scoped>
.k-status-pill {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  border-radius: var(--radius-full);
  font-family: var(--font-family);
  font-weight: var(--font-medium);
  white-space: nowrap;
  line-height: 1;
}

/* ─── Sizes ─── */

.k-status-pill--sm {
  padding: 3px 8px;
  font-size: var(--text-xs);
}

.k-status-pill--md {
  padding: 4px 10px;
  font-size: var(--text-sm);
}

/* ─── Dot ─── */

.k-status-pill__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

/* ─── Variants ─── */

.k-status-pill--ok {
  background: rgba(34, 197, 94, 0.12);
  color: var(--color-success);
}

.k-status-pill--ok .k-status-pill__dot {
  background: var(--color-success);
}

.k-status-pill--warn {
  background: rgba(245, 158, 11, 0.12);
  color: var(--color-warning);
}

.k-status-pill--warn .k-status-pill__dot {
  background: var(--color-warning);
}

.k-status-pill--bad {
  background: rgba(239, 68, 68, 0.12);
  color: var(--color-danger);
}

.k-status-pill--bad .k-status-pill__dot {
  background: var(--color-danger);
}

.k-status-pill--idle {
  background: rgba(139, 152, 165, 0.12);
  color: var(--color-muted);
}

.k-status-pill--idle .k-status-pill__dot {
  background: var(--color-muted);
}

.k-status-pill__text {
  text-transform: capitalize;
}
</style>
