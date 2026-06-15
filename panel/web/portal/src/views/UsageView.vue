<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { useUsageStore } from '@/stores/usage'
import KChart from '@koris/ui/KChart.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KDataTable from '@koris/ui/KDataTable.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import UsageGauge from '@/components/UsageGauge.vue'

const usageStore = useUsageStore()

onMounted(() => {
  usageStore.loadUsage()
})

const chartData = computed(() => usageStore.bandwidthChartData)
const sessions = computed(() => usageStore.sessions)
const isOnline = computed(() => usageStore.isOnline)
const usagePercent = computed(() => usageStore.usagePercent)
const totalUsageBytes = computed(() => usageStore.totalUsageBytes)
const maxDataBytes = computed(() => usageStore.maxDataBytes)

const sessionColumns = [
  { key: 'status', label: 'Status' },
  { key: 'framed_ip', label: 'IP Address' },
  { key: 'session_seconds', label: 'Duration' },
  { key: 'input_bytes', label: 'Download' },
  { key: 'output_bytes', label: 'Upload' },
  { key: 'total_bytes', label: 'Total' },
]

function formatBytes(value: number): string {
  if (value >= 1024 ** 4) return `${(value / 1024 ** 4).toFixed(2)} TB`
  if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(2)} GB`
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(2)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(2)} KB`
  return `${Math.round(value)} B`
}

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h) return `${h}h ${m}m`
  if (m) return `${m}m`
  return `${seconds}s`
}
</script>
<template>
  <div class="usage">
    <h1 class="usage__title">Usage</h1>

    <KSkeleton v-if="usageStore.loading && !usageStore.usage" type="card" :count="2" />

    <template v-else>
      <!-- Summary section -->
      <div class="usage__summary">
        <div class="usage__gauge-card">
          <div class="usage__gauge-header">
            <h3>Data Consumption</h3>
            <KStatusPill :status="isOnline ? 'active' : 'disabled'">
              {{ isOnline ? 'Online' : 'Offline' }}
            </KStatusPill>
          </div>
          <UsageGauge :percent="usagePercent" />
          <div class="usage__gauge-details">
            <span>{{ formatBytes(totalUsageBytes) }} used</span>
            <span>{{ maxDataBytes ? formatBytes(maxDataBytes) + ' limit' : 'Unlimited' }}</span>
          </div>
        </div>

        <div class="usage__stats-grid">
          <div class="usage__stat">
            <div class="usage__stat-label">Download</div>
            <div class="usage__stat-value">{{ formatBytes(usageStore.usage?.total_input_bytes || 0) }}</div>
          </div>
          <div class="usage__stat">
            <div class="usage__stat-label">Upload</div>
            <div class="usage__stat-value">{{ formatBytes(usageStore.usage?.total_output_bytes || 0) }}</div>
          </div>
          <div class="usage__stat">
            <div class="usage__stat-label">Active Sessions</div>
            <div class="usage__stat-value">{{ usageStore.activeSessions }}</div>
          </div>
          <div class="usage__stat">
            <div class="usage__stat-label">Remaining</div>
            <div class="usage__stat-value">{{ maxDataBytes ? formatBytes(usageStore.remainingBytes) : '∞' }}</div>
          </div>
        </div>
      </div>

      <!-- Bandwidth Chart -->
      <section class="usage__section" v-if="chartData.length">
        <h2 class="usage__section-title">Bandwidth Over Time</h2>
        <div class="usage__chart-container">
          <KChart
            type="area"
            :data="chartData"
            :height="250"
            :interactive="true"
            :animate="true"
            :gradient-fill="true"
            :options="{ showGrid: true }"
          />
        </div>
      </section>

      <!-- Sessions Table -->
      <section class="usage__section">
        <h2 class="usage__section-title">Recent Sessions</h2>

        <KEmptyState
          v-if="!sessions.length"
          title="No sessions recorded"
          description="Your connection sessions will appear here."
          icon="📡"
        />

        <KDataTable
          v-else
          :columns="sessionColumns"
          :data="sessions.slice(0, 20)"
          :loading="usageStore.loading"
        >
          <template #cell-status="{ row }">
            <KStatusPill :status="row.online ? 'active' : 'disabled'">
              {{ row.online ? 'Online' : 'Offline' }}
            </KStatusPill>
          </template>
          <template #cell-framed_ip="{ row }">
            {{ row.framed_ip || '—' }}
          </template>
          <template #cell-session_seconds="{ row }">
            {{ formatDuration(row.session_seconds) }}
          </template>
          <template #cell-input_bytes="{ row }">
            {{ formatBytes(row.input_bytes) }}
          </template>
          <template #cell-output_bytes="{ row }">
            {{ formatBytes(row.output_bytes) }}
          </template>
          <template #cell-total_bytes="{ row }">
            {{ formatBytes(row.total_bytes) }}
          </template>
        </KDataTable>
      </section>
    </template>
  </div>
</template>
<style scoped>
.usage__title {
  font-size: var(--text-2xl);
  font-weight: 700;
  margin-bottom: var(--space-6);
}
.usage__summary {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-4);
  margin-bottom: var(--space-6);
}
.usage__gauge-card {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.usage__gauge-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-4);
}
.usage__gauge-header h3 {
  font-size: var(--text-md);
  font-weight: 600;
}
.usage__gauge-details {
  display: flex;
  justify-content: space-between;
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-top: var(--space-3);
}
.usage__stats-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3);
}
.usage__stat {
  padding: var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.usage__stat-label {
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-bottom: var(--space-1);
}
.usage__stat-value {
  font-size: var(--text-md);
  font-weight: 600;
}
.usage__section {
  margin-bottom: var(--space-6);
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.usage__section-title {
  font-size: var(--text-md);
  font-weight: 600;
  margin-bottom: var(--space-4);
}
.usage__chart-container {
  width: 100%;
}
@media (max-width: 768px) {
  .usage__summary { grid-template-columns: 1fr; }
}
</style>
