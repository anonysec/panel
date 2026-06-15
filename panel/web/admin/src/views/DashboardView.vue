<script setup lang="ts">
import { computed } from 'vue'
import { useRealtimeStore } from '@/stores/realtime'
import { useCustomersStore } from '@/stores/customers'
import { useNodesStore } from '@/stores/nodes'
import KChart from '@koris/ui/KChart.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const realtime = useRealtimeStore()
const customers = useCustomersStore()
const nodes = useNodesStore()

customers.loadCustomers()
nodes.loadNodes()

const statCards = computed(() => [
  { label: 'Revenue', value: `$${realtime.stats.approved_payments}`, icon: '💰' },
  { label: 'Active Users', value: realtime.stats.active_customers, icon: '👥' },
  { label: 'Nodes Online', value: realtime.stats.nodes, icon: '🖥️' },
  { label: 'Open Tickets', value: realtime.stats.open_tickets, icon: '🎫' },
])

const trafficChartData = computed(() =>
  realtime.rxHistory.map((rx, i) => ({
    label: `${i}`,
    value: rx + (realtime.txHistory[i] || 0),
  }))
)

const userStatusData = computed(() => {
  const active = customers.list.filter(c => c.status === 'active').length
  const limited = customers.list.filter(c => c.status === 'limited').length
  const disabled = customers.list.filter(c => c.status === 'disabled').length
  const expired = customers.list.filter(c => c.status === 'expired').length
  return [
    { label: 'Active', value: active },
    { label: 'Limited', value: limited },
    { label: 'Disabled', value: disabled },
    { label: 'Expired', value: expired },
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
      <div v-for="stat in statCards" :key="stat.label" class="stat-card">
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
        <h4 class="panel-title">Traffic Overview</h4>
        <KChart
          v-if="trafficChartData.length > 2"
          type="area"
          :data="trafficChartData"
          :height="200"
          :options="{ gradientFill: true, showGrid: true }"
          :animate="true"
          :interactive="true"
        />
        <KSkeleton v-else variant="rect" :width="'100%'" :height="200" />
      </div>
      <div class="chart-panel chart-panel--donut">
        <h4 class="panel-title">User Status</h4>
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
      <h4 class="panel-title">Recent Users</h4>
      <div class="recent-table">
        <table class="mini-table" role="table">
          <thead>
            <tr>
              <th>Username</th>
              <th>Display Name</th>
              <th>Status</th>
              <th>Plan</th>
              <th>Created</th>
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
              <td colspan="5" class="text-muted text-center">No users yet</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Node Health + Live Sessions -->
    <section class="bottom-row">
      <div class="panel panel--nodes">
        <h4 class="panel-title">Node Health</h4>
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
          <p v-if="nodes.list.length === 0" class="text-muted">No nodes registered</p>
        </div>
      </div>

      <div class="panel panel--sessions">
        <h4 class="panel-title">Live Sessions</h4>
        <div class="sessions-list">
          <div v-for="session in realtime.liveSessions.slice(0, 8)" :key="session.id" class="session-row">
            <span class="session-row__user">{{ session.username }}</span>
            <span class="session-row__ip text-muted">{{ session.framed_ip }}</span>
            <span class="session-row__node text-muted">{{ session.node_name }}</span>
            <span class="session-row__traffic">{{ formatBytes(session.input_bytes + session.output_bytes) }}</span>
            <span class="session-row__duration text-muted">{{ formatDuration(session.session_seconds) }}</span>
          </div>
          <p v-if="realtime.liveSessions.length === 0" class="text-muted">No active sessions</p>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.dashboard { display: flex; flex-direction: column; gap: var(--space-6); }

.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-4); }
.stat-card { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.stat-card__icon { font-size: 1.5rem; }
.stat-card__body { display: flex; flex-direction: column; }
.stat-card__value { font-size: var(--text-xl); font-weight: var(--font-bold); color: var(--color-text); }
.stat-card__label { font-size: var(--text-xs); color: var(--color-muted); text-transform: uppercase; letter-spacing: var(--tracking-wider); }

.charts-row { display: grid; grid-template-columns: 2fr 1fr; gap: var(--space-4); }
.chart-panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }

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
</style>
