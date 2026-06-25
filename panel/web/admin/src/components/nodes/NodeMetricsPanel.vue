<script setup lang="ts">
import { computed } from 'vue'
import { useMetricsStore, type NodeMetricsState } from '@/stores/metrics'

const props = defineProps<{
  nodeId: number
}>()

const metricsStore = useMetricsStore()

const metrics = computed<NodeMetricsState | undefined>(() => {
  return metricsStore.nodes.get(props.nodeId)
})

const isOffline = computed(() => {
  if (!metrics.value) return true
  return metrics.value.status === 'offline' || metrics.value.status === 'stale'
})

// ─── Formatting Helpers ─────────────────────────────────────────────────────

function formatBandwidth(bps: number): string {
  if (bps < 1024) return `${bps} B/s`
  if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(1)} KB/s`
  if (bps < 1024 * 1024 * 1024) return `${(bps / (1024 * 1024)).toFixed(1)} MB/s`
  return `${(bps / (1024 * 1024 * 1024)).toFixed(2)} GB/s`
}

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function gaugeColor(value: number): string {
  if (value >= 90) return 'var(--color-danger)'
  if (value >= 70) return 'var(--color-warning)'
  return 'var(--color-success)'
}

function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleString()
}
</script>

<template>
  <div class="node-metrics-panel">
    <!-- Offline Banner -->
    <div v-if="isOffline" class="node-metrics-panel__offline-banner">
      <span class="node-metrics-panel__offline-icon">⚠️</span>
      <span>Node is {{ metrics?.status || 'offline' }}.</span>
      <span v-if="metrics?.lastUpdated" class="node-metrics-panel__offline-ts">
        Last data: {{ formatTimestamp(metrics.lastUpdated) }}
      </span>
    </div>

    <template v-if="metrics">
      <!-- Gauges Row -->
      <div class="node-metrics-panel__gauges">
        <div class="node-metrics-panel__gauge">
          <div class="node-metrics-panel__gauge-label">CPU</div>
          <div class="node-metrics-panel__progress-bar">
            <div
              class="node-metrics-panel__progress-fill"
              :style="{ width: `${metrics.cpu}%`, backgroundColor: gaugeColor(metrics.cpu) }"
            />
          </div>
          <div class="node-metrics-panel__gauge-value">{{ metrics.cpu.toFixed(1) }}%</div>
        </div>

        <div class="node-metrics-panel__gauge">
          <div class="node-metrics-panel__gauge-label">RAM</div>
          <div class="node-metrics-panel__progress-bar">
            <div
              class="node-metrics-panel__progress-fill"
              :style="{ width: `${metrics.ram}%`, backgroundColor: gaugeColor(metrics.ram) }"
            />
          </div>
          <div class="node-metrics-panel__gauge-value">{{ metrics.ram.toFixed(1) }}%</div>
        </div>

        <div class="node-metrics-panel__gauge">
          <div class="node-metrics-panel__gauge-label">Disk</div>
          <div class="node-metrics-panel__progress-bar">
            <div
              class="node-metrics-panel__progress-fill"
              :style="{ width: `${metrics.disk}%`, backgroundColor: gaugeColor(metrics.disk) }"
            />
          </div>
          <div class="node-metrics-panel__gauge-value">{{ metrics.disk.toFixed(1) }}%</div>
        </div>
      </div>

      <!-- Stats Row -->
      <div class="node-metrics-panel__stats">
        <div class="node-metrics-panel__stat">
          <span class="node-metrics-panel__stat-label">RX</span>
          <span class="node-metrics-panel__stat-value">{{ formatBandwidth(metrics.rxBps) }}</span>
        </div>
        <div class="node-metrics-panel__stat">
          <span class="node-metrics-panel__stat-label">TX</span>
          <span class="node-metrics-panel__stat-value">{{ formatBandwidth(metrics.txBps) }}</span>
        </div>
        <div class="node-metrics-panel__stat">
          <span class="node-metrics-panel__stat-label">Sessions</span>
          <span class="node-metrics-panel__stat-value">{{ metrics.sessions }}</span>
        </div>
        <div class="node-metrics-panel__stat">
          <span class="node-metrics-panel__stat-label">Uptime</span>
          <span class="node-metrics-panel__stat-value">{{ formatUptime(metrics.uptime) }}</span>
        </div>
      </div>
    </template>

    <div v-else class="node-metrics-panel__no-data">
      No metrics data available
    </div>
  </div>
</template>

<style scoped>
.node-metrics-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.node-metrics-panel__offline-banner {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  background: color-mix(in srgb, var(--color-warning) 12%, transparent);
  border: 1px solid var(--color-warning);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  color: var(--color-warning);
}

.node-metrics-panel__offline-icon {
  flex-shrink: 0;
}

.node-metrics-panel__offline-ts {
  margin-left: auto;
  font-size: var(--text-xs);
  color: var(--color-muted);
}

.node-metrics-panel__gauges {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-3);
}

.node-metrics-panel__gauge {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.node-metrics-panel__gauge-label {
  font-size: var(--text-xs);
  font-weight: var(--font-medium);
  color: var(--color-muted);
  text-transform: uppercase;
}

.node-metrics-panel__progress-bar {
  height: 8px;
  background: var(--color-border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.node-metrics-panel__progress-fill {
  height: 100%;
  border-radius: var(--radius-sm);
  transition: width 0.3s ease, background-color 0.3s ease;
}

.node-metrics-panel__gauge-value {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.node-metrics-panel__stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: var(--space-3);
  padding-top: var(--space-2);
  border-top: 1px solid var(--color-border);
}

.node-metrics-panel__stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.node-metrics-panel__stat-label {
  font-size: var(--text-xs);
  color: var(--color-muted);
}

.node-metrics-panel__stat-value {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-text);
  font-family: monospace;
}

.node-metrics-panel__no-data {
  padding: var(--space-4);
  text-align: center;
  color: var(--color-muted);
  font-size: var(--text-sm);
}
</style>
