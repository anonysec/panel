<template>
  <div class="k-skeleton-group" :aria-busy="true" aria-label="Loading content">
    <div
      v-for="i in resolvedCount"
      :key="i"
      :class="['k-skeleton', `k-skeleton--${resolvedVariant}`]"
      :style="skeletonStyle"
      role="status"
      aria-hidden="true"
    >
      <span class="k-skeleton__shimmer" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface KSkeletonProps {
  variant?: 'text' | 'card' | 'circle' | 'table-row' | 'rect'
  width?: string
  height?: string | number
  count?: number | string
}

const props = withDefaults(defineProps<KSkeletonProps>(), {
  variant: 'text',
  count: 1,
})

const resolvedVariant = computed(() => {
  if (props.variant === 'rect') return 'text'
  return props.variant
})

const resolvedCount = computed(() => {
  return typeof props.count === 'string' ? parseInt(props.count, 10) || 1 : props.count
})

const skeletonStyle = computed(() => {
  const style: Record<string, string> = {}
  if (props.width) style.width = props.width
  if (props.height) style.height = typeof props.height === 'number' ? `${props.height}px` : props.height
  return style
})
</script>

<style scoped>
.k-skeleton-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.k-skeleton {
  position: relative;
  overflow: hidden;
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
}

/* ─── Variants ─── */

.k-skeleton--text {
  height: 14px;
  width: 100%;
  border-radius: var(--radius-sm);
}

.k-skeleton--card {
  height: 120px;
  width: 100%;
  border-radius: var(--radius-lg);
}

.k-skeleton--circle {
  width: 40px;
  height: 40px;
  border-radius: var(--radius-full);
}

.k-skeleton--table-row {
  height: 44px;
  width: 100%;
  border-radius: var(--radius-sm);
}

/* ─── Shimmer Animation ─── */

.k-skeleton__shimmer {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    90deg,
    transparent 0%,
    rgba(255, 255, 255, 0.04) 40%,
    rgba(255, 255, 255, 0.08) 50%,
    rgba(255, 255, 255, 0.04) 60%,
    transparent 100%
  );
  animation: k-skeleton-shimmer 1.8s infinite ease-in-out;
}

@keyframes k-skeleton-shimmer {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(100%);
  }
}

/* Respect reduced motion */
@media (prefers-reduced-motion: reduce) {
  .k-skeleton__shimmer {
    animation: none;
    background: rgba(255, 255, 255, 0.04);
  }
}
</style>
