<script setup lang="ts">
import { computed } from 'vue'
import { useUsageStore } from '@/stores/usage'
import { formatBytes } from '@/composables/useUsageDisplay'
import { useFreshData } from '@koris/composables/useFreshData'
import KChart from '@koris/ui/KChart.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KDataTable from '@koris/ui/KDataTable.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KUsageBar from '@koris/ui/KUsageBar.vue'

const usageStore = useUsageStore()

useFreshData(() => usageStore.loadUsage())

const chartData = computed(() => usageStore.bandwidthChartData)
const sessions = computed(() => usageStore.sessions)
const isOnline = computed(() => usageStore.isOnline)
const usagePercent = computed(() => usageStore.usagePercent)
const totalUsageBytes = computed(() => usageStore.totalUsageBytes)
const maxDataBytes = computed(() => usageStore.maxDataBytes)
const activeSessions = computed(() => usageStore.activeSessions)
const connectionLimit = computed(() => usageStore.connectionLimit)

const connectionLimitDisplay = computed(() => {
  if (!connectionLimit.value || connectionLimit.value === 0) return 'Unlimited'
  return String(connectionLimit.value)
})

const sessionColumns = [
  { key: 'status', label: 'Status' },
  { key: 'framed_ip', label: 'IP Address' },
  { key: 'session_seconds', label: 'Duration' },
  { key: 'input_bytes', label: 'Download' },
  { key: 'output_bytes', label: 'Upload' },
  { key: 'total_bytes', label: 'Total' },
]

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
          <KUsageBar :used="totalUsageBytes" :limit="maxDataBytes || 0" />
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
            <div class="usage__stat-value">{{ activeSessions }}</div>
          </div>
          <div class="usage__stat">
            <div class="usage__stat-label">Remaining</div>
            <div class="usage__stat-value">{{ maxDataBytes ? formatBytes(usageStore.remainingBytes) : '∞' }}</div>
          </div>
        </div>
      </div>

      <!-- Connection Info section -->
      <section class="usage__connection-info">
        <h2 class="usage__section-title">Connection Info</h2>
        <div class="usage__connection-cards">
          <div class="usage__connection-card">
            <div class="usage__connection-card-icon">
              <span class="usage__connection-icon">&#x1F4E1;</span>
            </div>
            <div class="usage__connection-card-content">
              <div class="usage__connection-card-label">Active Sessions</div>
              <div class="usage__connection-card-value">{{ activeSessions }}</div>
              <div class="usage__connection-card-sub">Currently connected device{{ activeSessions !== 1 ? 's' : '' }}</div>
            </div>
          </div>
          <div class="usage__connection-card">
            <div class="usage__connection-card-icon">
              <span class="usage__connection-icon">&#x1F512;</span>
            </div>
            <div class="usage__connection-card-content">
              <div class="usage__connection-card-label">Connection Limit</div>
              <div class="usage__connection-card-value">{{ connectionLimitDisplay }}</div>
              <div class="usage__connection-card-sub">
                <template v-if="connectionLimit > 0">
                  {{ activeSessions }} of {{ connectionLimit }} used
                </template>
                <template v-else>
                  No concurrent session restriction
                </template>
              </div>
            </div>
          </div>
        </div>
      </section>

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
.usage__connection-info {
  margin-bottom: var(--space-6);
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.usage__connection-cards {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-4);
}
.usage__connection-card {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-4);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}
.usage__connection-card-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: var(--radius-md);
  background: var(--color-surface);
  flex-shrink: 0;
}
.usage__connection-icon {
  font-size: var(--text-lg);
}
.usage__connection-card-content {
  flex: 1;
  min-width: 0;
}
.usage__connection-card-label {
  font-size: var(--text-xs);
  color: var(--color-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: var(--space-1);
}
.usage__connection-card-value {
  font-size: var(--text-xl);
  font-weight: 700;
  margin-bottom: var(--space-1);
}
.usage__connection-card-sub {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
@media (max-width: 768px) {
  .usage__summary { grid-template-columns: 1fr; }
  .usage__connection-cards { grid-template-columns: 1fr; }
}
</style>
