<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  percent: number
  size?: number
  strokeWidth?: number
}

const props = withDefaults(defineProps<Props>(), {
  size: 160,
  strokeWidth: 12,
})

const radius = computed(() => (props.size - props.strokeWidth) / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)
const dashOffset = computed(() => circumference.value * (1 - props.percent / 100))
const center = computed(() => props.size / 2)

const gaugeColor = computed(() => {
  if (props.percent >= 90) return 'var(--color-danger)'
  if (props.percent >= 70) return 'var(--color-warning)'
  return 'var(--color-primary)'
})
</script>
<template>
  <div class="usage-gauge" :style="{ width: `${size}px`, height: `${size}px` }">
    <svg :width="size" :height="size" class="usage-gauge__svg">
      <!-- Background circle -->
      <circle
        :cx="center"
        :cy="center"
        :r="radius"
        fill="none"
        :stroke-width="strokeWidth"
        class="usage-gauge__track"
      />
      <!-- Progress circle -->
      <circle
        :cx="center"
        :cy="center"
        :r="radius"
        fill="none"
        :stroke="gaugeColor"
        :stroke-width="strokeWidth"
        :stroke-dasharray="circumference"
        :stroke-dashoffset="dashOffset"
        stroke-linecap="round"
        class="usage-gauge__progress"
        :style="{ transform: 'rotate(-90deg)', transformOrigin: 'center' }"
      />
    </svg>
    <div class="usage-gauge__label">
      <span class="usage-gauge__value">{{ percent }}%</span>
      <span class="usage-gauge__text">used</span>
    </div>
  </div>
</template>
<style scoped>
.usage-gauge {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto;
}
.usage-gauge__svg {
  display: block;
}
.usage-gauge__track {
  stroke: var(--color-border);
}
.usage-gauge__progress {
  transition: stroke-dashoffset 0.6s ease;
}
.usage-gauge__label {
  position: absolute;
  display: flex;
  flex-direction: column;
  align-items: center;
}
.usage-gauge__value {
  font-size: var(--text-xl);
  font-weight: 700;
}
.usage-gauge__text {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
</style>
