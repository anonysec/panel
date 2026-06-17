<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useRealtimeStore } from '@/stores/realtime'
import { useCustomersStore } from '@/stores/customers'
import { useNodesStore } from '@/stores/nodes'
import { useI18n } from '@koris/composables/useI18n'
import KChart from '@koris/ui/KChart.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const { t } = useI18n()
const router = useRouter()
const realtime = useRealtimeStore()
const customers = useCustomersStore()
const nodes = useNodesStore()

customers.loadCustomers()
nodes.loadNodes()

const statCards = computed(() => [
  { label: t('stat.revenue'), value: `$${realtime.stats.approved_payments}`, icon: '💰', route: 'payments' },
  { label: t('stat.active_users'), value: realtime.stats.active_customers, icon: '👥', route: 'customers' },
  { label: t('stat.nodes_online'), value: realtime.stats.nodes, icon: '🖥️', route: 'nodes' },
  { label: t('stat.open_tickets'), value: realtime.stats.open_tickets, icon: '🎫', route: 'tickets' },
])

function handleStatClick(routeName: string) {
  router.push({ name: routeName })
}

const trafficChartData = computed(() => {
  // Compute cumulative data usage (bytes) from bps history.
  // Each sample in rxHistory/txHistory is a bps value pushed every ~3 seconds.
  // To convert to bytes transferred per interval: (bps * 3) / 8
  let cumulative = 0
  return realtime.rxHistory.map((rx, i) => {
    const intervalBps = rx + (realtime.txHistory[i] || 0)
    const intervalBytes = (intervalBps * 3) / 8
    cumulative += intervalBytes
    return {
      label: `${i}`,
      value: cumulative,
    }
  })
})

/** Total data transferred computed from live sessions */
const totalDownloaded = computed(() =>
  realtime.liveSessions.reduce((sum, s) => sum + (s.input_bytes || 0), 0)
)
const totalUploaded = computed(() =>
  realtime.liveSessions.reduce((sum, s) => sum + (s.output_bytes || 0), 0)
)

const userStatusData = computed(() => {
  const active = customers.list.filter(c => c.status === 'active').length
  const limited = customers.list.filter(c => c.status === 'limited').length
  const disabled = customers.list.filter(c => c.status === 'disabled').length
  const expired = customers.list.filter(c => c.status === 'expired').length
  return [
    { label: t('status.active'), value: active },
    { label: t('status.limited'), value: limited },
    { label: t('status.disabled'), value: disabled },
    { label: t('status.expired'), value: expired },
  ]
})

const recentUsers = computed(() => customers.list.slice(0, 6))

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`
  return `${(bytes / 1073741824).toFixed(2)} GB`
}

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}
</script>

<template>
  <div class="page dashboard">
    <!-- Stats Grid -->
    <section class="stats-grid" aria-label="Overview statistics">
      <div v-for="stat in statCards" :key="stat.label" class="stat-card stat-card--clickable" @click="handleStatClick(stat.route)">
        <span class="stat-card__icon">{{ stat.icon }}</span>
        <div class="stat-card__body">
          <span class="stat-card__value">{{ stat.value }}</span>
          <span class="stat-card__label">{{ stat.label }}</span>
        </div>
      </div>
    </section>

    <!-- Charts Row -->
    <section class="charts-row">
      <div class="chart-panel chart-panel--traffic">
        <h4 class="panel-title">{{ t('dashboard.data_usage') }}</h4>
        <div v-if="trafficChartData.length > 2">
          <KChart
            type="area"
            :data="trafficChartData"
            :height="200"
            :options="{ gradientFill: true, showGrid: true }"
            :animate="true"
            :interactive="true"
          />
        </div>
        <div v-else class="traffic-fallback">
          <div class="traffic-live">
            <div class="traffic-stat">
              <span class="traffic-stat__label">{{ t('dashboard.total_downloaded') }}</span>
              <span class="traffic-stat__value">{{ formatBytes(totalDownloaded) }}</span>
            </div>
            <div class="traffic-stat">
              <span class="traffic-stat__label">{{ t('dashboard.total_uploaded') }}</span>
              <span class="traffic-stat__value">{{ formatBytes(totalUploaded) }}</span>
            </div>
          </div>
          <p class="traffic-note">{{ t('dashboard.chart_loading') }}</p>
        </div>
      </div>
      <div class="chart-panel chart-panel--donut">
        <h4 class="panel-title">{{ t('dashboard.user_status') }}</h4>
        <KChart
          v-if="customers.list.length > 0"
          type="donut"
          :data="userStatusData"
          :height="200"
          :animate="true"
        />
        <KSkeleton v-else variant="rect" :width="'100%'" :height="200" />
      </div>
    </section>

    <!-- Recent Users -->
    <section class="panel">
      <h4 class="panel-title">{{ t('dashboard.recent_users') }}</h4>
      <div class="recent-table">
        <table class="mini-table" role="table">
          <thead>
            <tr>
              <th>{{ t('user.username') }}</th>
              <th>{{ t('user.display_name') }}</th>
              <th>{{ t('user.status') }}</th>
              <th>{{ t('user.plan') }}</th>
              <th>{{ t('user.created') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in recentUsers" :key="user.id">
              <td class="text-primary">{{ user.username }}</td>
              <td>{{ user.display_name }}</td>
              <td><KStatusPill :status="user.status" size="sm" /></td>
              <td>{{ user.plan }}</td>
              <td class="text-muted">{{ user.created_at?.slice(0, 10) }}</td>
            </tr>
            <tr v-if="recentUsers.length === 0">
              <td colspan="5" class="text-muted text-center">{{ t('empty.no_users') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Node Health + Live Sessions -->
    <section class="bottom-row">
      <div class="panel panel--nodes">
        <h4 class="panel-title">{{ t('dashboard.node_health') }}</h4>
        <div class="node-cards">
          <div v-for="node in nodes.list" :key="node.id" class="node-health-card">
            <div class="node-health-card__header">
              <span class="node-health-card__name">{{ node.name }}</span>
              <KStatusPill :status="node.status" size="sm" />
            </div>
            <div class="node-health-card__metrics">
              <div class="metric-bar">
                <span class="metric-bar__label">CPU</span>
                <div class="metric-bar__track">
                  <div class="metric-bar__fill" :style="{ width: `${node.status_metrics?.cpu_percent ?? 0}%` }" />
                </div>
                <span class="metric-bar__value">{{ node.status_metrics?.cpu_percent ?? 0 }}%</span>
              </div>
              <div class="metric-bar">
                <span class="metric-bar__label">RAM</span>
                <div class="metric-bar__track">
                  <div class="metric-bar__fill metric-bar__fill--accent" :style="{ width: `${node.status_metrics?.ram_percent ?? 0}%` }" />
                </div>
                <span class="metric-bar__value">{{ node.status_metrics?.ram_percent ?? 0 }}%</span>
              </div>
            </div>
          </div>
          <p v-if="nodes.list.length === 0" class="text-muted">{{ t('empty.no_nodes') }}</p>
        </div>
      </div>

      <div class="panel panel--sessions">
        <h4 class="panel-title">{{ t('dashboard.live_sessions') }}</h4>
        <div class="sessions-list">
          <div v-for="session in realtime.liveSessions.slice(0, 8)" :key="session.id" class="session-row">
            <span class="session-row__user">{{ session.username }}</span>
            <span class="session-row__ip text-muted">{{ session.framed_ip }}</span>
            <span class="session-row__node text-muted">{{ session.node_name }}</span>
            <span class="session-row__traffic">{{ formatBytes(session.input_bytes + session.output_bytes) }}</span>
            <span class="session-row__duration text-muted">{{ formatDuration(session.session_seconds) }}</span>
          </div>
          <p v-if="realtime.liveSessions.length === 0" class="text-muted">{{ t('empty.no_sessions') }}</p>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.dashboard { display: flex; flex-direction: column; gap: var(--space-6); }

.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-4); }
.stat-card { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.stat-card--clickable { cursor: pointer; transition: transform 0.15s, border-color 0.15s; }
.stat-card--clickable:hover { transform: translateY(-2px); border-color: rgba(37, 99, 235, 0.3); }
.stat-card__icon { font-size: 1.5rem; }
.stat-card__body { display: flex; flex-direction: column; }
.stat-card__value { font-size: var(--text-xl); font-weight: var(--font-bold); color: var(--color-text); }
.stat-card__label { font-size: var(--text-xs); color: var(--color-muted); text-transform: uppercase; letter-spacing: var(--tracking-wider); }

.charts-row { display: grid; grid-template-columns: 2fr 1fr; gap: var(--space-4); }
.chart-panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }

.traffic-fallback { display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 200px; gap: var(--space-4); }
.traffic-live { display: flex; gap: var(--space-6); }
.traffic-stat { display: flex; flex-direction: column; align-items: center; gap: var(--space-1); }
.traffic-stat__label { font-size: var(--text-xs); color: var(--color-muted); text-transform: uppercase; letter-spacing: var(--tracking-wider); }
.traffic-stat__value { font-size: var(--text-xl); font-weight: var(--font-bold); color: var(--color-text); }
.traffic-note { font-size: var(--text-xs); color: var(--color-muted); margin: 0; }

.panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.panel-title { margin: 0 0 var(--space-3); font-size: var(--text-sm); font-weight: var(--font-semibold); color: var(--color-text); }

.mini-table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
.mini-table th { text-align: left; padding: var(--space-2) var(--space-3); color: var(--color-muted); font-size: var(--text-xs); text-transform: uppercase; letter-spacing: var(--tracking-wider); border-bottom: 1px solid var(--color-border); }
.mini-table td { padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); color: var(--color-text); }

.bottom-row { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-4); }
.node-cards { display: flex; flex-direction: column; gap: var(--space-3); }
.node-health-card { padding: var(--space-3); background: var(--color-surface-2); border-radius: var(--radius-md); }
.node-health-card__header { display: flex; justify-content: space-between; align-items: center; margin-bottom: var(--space-2); }
.node-health-card__name { font-weight: var(--font-medium); font-size: var(--text-sm); }
.node-health-card__metrics { display: flex; flex-direction: column; gap: var(--space-2); }

.metric-bar { display: flex; align-items: center; gap: var(--space-2); }
.metric-bar__label { font-size: var(--text-xs); color: var(--color-muted); width: 32px; }
.metric-bar__track { flex: 1; height: 6px; background: var(--color-border); border-radius: 3px; overflow: hidden; }
.metric-bar__fill { height: 100%; background: var(--color-primary); border-radius: 3px; transition: width 0.3s ease; }
.metric-bar__fill--accent { background: var(--color-accent); }
.metric-bar__value { font-size: var(--text-xs); color: var(--color-muted); width: 32px; text-align: right; }

.sessions-list { display: flex; flex-direction: column; gap: var(--space-2); }
.session-row { display: grid; grid-template-columns: 1.5fr 1fr 1fr 1fr 0.8fr; gap: var(--space-2); padding: var(--space-2) 0; border-bottom: 1px solid var(--color-border); font-size: var(--text-sm); align-items: center; }
.session-row__user { font-weight: var(--font-medium); color: var(--color-primary); }

.text-primary { color: var(--color-primary); }
.text-muted { color: var(--color-muted); }
.text-center { text-align: center; }

@media (max-width: 768px) {
  .charts-row, .bottom-row { grid-template-columns: 1fr; }
}

/* RTL support */
[data-dir="rtl"] .stat-card__body { text-align: right; }
[data-dir="rtl"] .stat-card__label { text-align: right; }
[data-dir="rtl"] .mini-table th { text-align: right; }
[data-dir="rtl"] .mini-table td { text-align: right; }
[data-dir="rtl"] .metric-bar__value { text-align: left; }
[data-dir="rtl"] .metric-bar__label { text-align: right; }
[data-dir="rtl"] .session-row { direction: rtl; }
[data-dir="rtl"] .panel-title { text-align: right; }
[data-dir="rtl"] .charts-row { direction: rtl; }
[data-dir="rtl"] .bottom-row { direction: rtl; }
[data-dir="rtl"] .traffic-stat__label { text-align: center; }
</style>
