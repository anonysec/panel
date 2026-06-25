<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'

export interface BandwidthDataPoint {
  ts: string
  rx_bps: number
  tx_bps: number
}

const props = withDefaults(defineProps<{
  data: BandwidthDataPoint[]
  rangeOptions?: string[]
}>(), {
  rangeOptions: () => ['1h', '6h', '24h'],
})

const emit = defineEmits<{
  (e: 'range-change', range: string): void
}>()

const activeRange = ref(props.rangeOptions[0] || '1h')

// ─── Responsive Width ───────────────────────────────────────────────────────
const containerRef = ref<HTMLElement | null>(null)
const chartWidth = ref(600)
const chartHeight = 200
const padding = { top: 20, right: 16, bottom: 30, left: 56 }

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  if (containerRef.value) {
    chartWidth.value = containerRef.value.clientWidth
    resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) {
        chartWidth.value = entry.contentRect.width
      }
    })
    resizeObserver.observe(containerRef.value)
  }
})

onUnmounted(() => {
  resizeObserver?.disconnect()
})

// ─── Formatting ─────────────────────────────────────────────────────────────

function formatBps(bps: number): string {
  if (bps < 1024) return `${bps} B/s`
  if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(0)} KB/s`
  if (bps < 1024 * 1024 * 1024) return `${(bps / (1024 * 1024)).toFixed(1)} MB/s`
  return `${(bps / (1024 * 1024 * 1024)).toFixed(2)} GB/s`
}

// ─── Chart Calculations ─────────────────────────────────────────────────────

const innerWidth = computed(() => chartWidth.value - padding.left - padding.right)
const innerHeight = computed(() => chartHeight - padding.top - padding.bottom)

const maxValue = computed(() => {
  if (props.data.length === 0) return 1
  const allValues = props.data.flatMap(d => [d.rx_bps, d.tx_bps])
  return Math.max(...allValues, 1)
})

function xScale(index: number): number {
  if (props.data.length <= 1) return padding.left
  return padding.left + (index / (props.data.length - 1)) * innerWidth.value
}

function yScale(value: number): number {
  return padding.top + innerHeight.value - (value / maxValue.value) * innerHeight.value
}

const rxPath = computed(() => buildPath('rx_bps'))
const txPath = computed(() => buildPath('tx_bps'))

function buildPath(key: 'rx_bps' | 'tx_bps'): string {
  if (props.data.length === 0) return ''
  return props.data
    .map((d, i) => `${i === 0 ? 'M' : 'L'} ${xScale(i).toFixed(1)} ${yScale(d[key]).toFixed(1)}`)
    .join(' ')
}

// Y-axis ticks
const yTicks = computed(() => {
  const max = maxValue.value
  const steps = 4
  return Array.from({ length: steps + 1 }, (_, i) => {
    const val = (max / steps) * i
    return { value: val, y: yScale(val), label: formatBps(val) }
  })
})

// ─── Range Selection ────────────────────────────────────────────────────────

function selectRange(range: string) {
  activeRange.value = range
  emit('range-change', range)
}
</script>

<template>
  <div ref="containerRef" class="bandwidth-chart">
    <!-- Range Selector -->
    <div class="bandwidth-chart__range-selector">
      <button
        v-for="range in rangeOptions"
        :key="range"
        class="bandwidth-chart__range-btn"
        :class="{ 'bandwidth-chart__range-btn--active': activeRange === range }"
        @click="selectRange(range)"
      >
        {{ range }}
      </button>
    </div>

    <!-- Legend -->
    <div class="bandwidth-chart__legend">
      <span class="bandwidth-chart__legend-item">
        <span class="bandwidth-chart__legend-color bandwidth-chart__legend-color--rx"></span>
        RX
      </span>
      <span class="bandwidth-chart__legend-item">
        <span class="bandwidth-chart__legend-color bandwidth-chart__legend-color--tx"></span>
        TX
      </span>
    </div>

    <!-- SVG Chart -->
    <svg
      :width="chartWidth"
      :height="chartHeight"
      :viewBox="`0 0 ${chartWidth} ${chartHeight}`"
      class="bandwidth-chart__svg"
      role="img"
      aria-label="Bandwidth chart"
    >
      <!-- Y-axis grid lines and labels -->
      <template v-for="tick in yTicks" :key="tick.value">
        <line
          :x1="padding.left"
          :y1="tick.y"
          :x2="chartWidth - padding.right"
          :y2="tick.y"
          class="bandwidth-chart__grid-line"
        />
        <text
          :x="padding.left - 8"
          :y="tick.y"
          text-anchor="end"
          dominant-baseline="middle"
          class="bandwidth-chart__axis-label"
        >
          {{ tick.label }}
        </text>
      </template>

      <!-- RX line (blue) -->
      <path
        v-if="rxPath"
        :d="rxPath"
        fill="none"
        stroke="var(--color-accent)"
        stroke-width="2"
        stroke-linejoin="round"
        stroke-linecap="round"
      />

      <!-- TX line (green) -->
      <path
        v-if="txPath"
        :d="txPath"
        fill="none"
        stroke="var(--color-success)"
        stroke-width="2"
        stroke-linejoin="round"
        stroke-linecap="round"
      />

      <!-- Empty state -->
      <text
        v-if="data.length === 0"
        :x="chartWidth / 2"
        :y="chartHeight / 2"
        text-anchor="middle"
        class="bandwidth-chart__empty-text"
      >
        No data available
      </text>
    </svg>
  </div>
</template>

<style scoped>
.bandwidth-chart {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  width: 100%;
}

.bandwidth-chart__range-selector {
  display: flex;
  gap: var(--space-1);
}

.bandwidth-chart__range-btn {
  padding: var(--space-1) var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-muted);
  font-size: var(--text-xs);
  font-weight: var(--font-medium);
  cursor: pointer;
  transition: all 0.15s ease;
}

.bandwidth-chart__range-btn:hover {
  border-color: var(--color-accent);
  color: var(--color-text);
}

.bandwidth-chart__range-btn--active {
  background: var(--color-accent);
  border-color: var(--color-accent);
  color: #fff;
}

.bandwidth-chart__legend {
  display: flex;
  gap: var(--space-3);
  font-size: var(--text-xs);
  color: var(--color-muted);
}

.bandwidth-chart__legend-item {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.bandwidth-chart__legend-color {
  display: inline-block;
  width: 12px;
  height: 3px;
  border-radius: 2px;
}

.bandwidth-chart__legend-color--rx {
  background: var(--color-accent);
}

.bandwidth-chart__legend-color--tx {
  background: var(--color-success);
}

.bandwidth-chart__svg {
  display: block;
  width: 100%;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.bandwidth-chart__grid-line {
  stroke: var(--color-border);
  stroke-width: 0.5;
  stroke-dasharray: 4 2;
}

.bandwidth-chart__axis-label {
  fill: var(--color-muted);
  font-size: 10px;
  font-family: var(--font-family);
}

.bandwidth-chart__empty-text {
  fill: var(--color-muted);
  font-size: var(--text-sm);
  font-family: var(--font-family);
}
</style>
