<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  value: number
  label: string
  threshold: number
  unit?: string
}>(), {
  unit: '%',
})

// ─── SVG Arc Calculations ───────────────────────────────────────────────────
const size = 120
const strokeWidth = 10
const radius = (size - strokeWidth) / 2
const circumference = 2 * Math.PI * radius

const clampedValue = computed(() => Math.min(Math.max(props.value, 0), 100))

const strokeDashoffset = computed(() => {
  return circumference - (clampedValue.value / 100) * circumference
})

const color = computed(() => {
  if (props.value > props.threshold) return 'var(--color-danger)'
  if (props.value > props.threshold * 0.8) return 'var(--color-warning)'
  return 'var(--color-success)'
})
</script>

<template>
  <div class="metrics-gauge">
    <svg
      :width="size"
      :height="size"
      :viewBox="`0 0 ${size} ${size}`"
      class="metrics-gauge__svg"
      role="img"
      :aria-label="`${label}: ${value}${unit}`"
    >
      <!-- Background track -->
      <circle
        class="metrics-gauge__track"
        :cx="size / 2"
        :cy="size / 2"
        :r="radius"
        fill="none"
        :stroke-width="strokeWidth"
      />
      <!-- Filled arc -->
      <circle
        class="metrics-gauge__fill"
        :cx="size / 2"
        :cy="size / 2"
        :r="radius"
        fill="none"
        :stroke="color"
        :stroke-width="strokeWidth"
        stroke-linecap="round"
        :stroke-dasharray="circumference"
        :stroke-dashoffset="strokeDashoffset"
        transform="rotate(-90 60 60)"
      />
      <!-- Value text -->
      <text
        :x="size / 2"
        :y="size / 2"
        text-anchor="middle"
        dominant-baseline="central"
        class="metrics-gauge__value-text"
      >
        <tspan class="metrics-gauge__number">{{ Math.round(value) }}</tspan>
        <tspan class="metrics-gauge__unit">{{ unit }}</tspan>
      </text>
    </svg>
    <span class="metrics-gauge__label">{{ label }}</span>
  </div>
</template>

<style scoped>
.metrics-gauge {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-2);
}

.metrics-gauge__svg {
  display: block;
}

.metrics-gauge__track {
  stroke: var(--color-border);
}

.metrics-gauge__fill {
  transition: stroke-dashoffset 0.4s ease, stroke 0.3s ease;
}

.metrics-gauge__value-text {
  fill: var(--color-text);
  font-family: var(--font-family);
}

.metrics-gauge__number {
  font-size: 1.5rem;
  font-weight: var(--font-semibold);
}

.metrics-gauge__unit {
  font-size: 0.75rem;
  fill: var(--color-muted);
}

.metrics-gauge__label {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
</style>
